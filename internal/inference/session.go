package inference

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	ort "github.com/yalue/onnxruntime_go"
)

// DefaultIdleUnload is how long a model may sit unused before its weights are freed.
const DefaultIdleUnload = 5 * time.Minute

// idleUnload is the unload delay set by SetIdleUnload; 0 or less keeps sessions loaded.
var idleUnload atomic.Int64

func init() { idleUnload.Store(int64(DefaultIdleUnload)) }

// SetIdleUnload sets how long a model may sit unused before its ORT session is destroyed; d <= 0 never unloads.
func SetIdleUnload(d time.Duration) { idleUnload.Store(int64(d)) }

var errSessionClosed = errors.New("ort session closed")

// idleSession is an ORT session that destroys itself after sitting unused and is rebuilt on the next Run.
type idleSession struct {
	path            string
	inputs, outputs []string

	mu     sync.RWMutex // Run holds it shared; load, expire and Close hold it exclusively
	s      *ort.DynamicAdvancedSession
	closed bool
	timer  *time.Timer
	last   atomic.Int64 // unix nanos of the latest Run
}

// newIdleSession creates the session now, so a bad model fails at startup rather than on first use.
func newIdleSession(path string, inputs, outputs []string) (*idleSession, error) {
	s := &idleSession{path: path, inputs: inputs, outputs: outputs}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *idleSession) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errSessionClosed
	}
	if s.s != nil {
		return nil
	}
	sess, err := newDynamicSession(s.path, s.inputs, s.outputs)
	if err != nil {
		return fmt.Errorf("create ort session: %w", err)
	}
	if s.timer != nil {
		slog.Info("model reloaded", "model", filepath.Base(s.path))
	}
	s.s = sess
	s.last.Store(time.Now().UnixNano())
	if d := time.Duration(idleUnload.Load()); d > 0 {
		if s.timer == nil {
			s.timer = time.AfterFunc(d, s.expire)
		} else {
			s.timer.Reset(d)
		}
	}
	return nil
}

// Run runs the session, reloading it first if it was unloaded.
func (s *idleSession) Run(inputs, outputs []ort.Value) error {
	s.mu.RLock()
	for s.s == nil {
		s.mu.RUnlock()
		if err := s.load(); err != nil {
			return err
		}
		s.mu.RLock()
	}
	defer s.mu.RUnlock()
	err := s.s.Run(inputs, outputs)
	s.last.Store(time.Now().UnixNano())
	return err
}

// expire frees the session if no Run touched it for the idle delay, else re-arms for the remainder.
func (s *idleSession) expire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := time.Duration(idleUnload.Load())
	if s.s == nil || s.closed || d <= 0 {
		return
	}
	if idle := time.Since(time.Unix(0, s.last.Load())); idle < d {
		s.timer.Reset(d - idle)
		return
	}
	s.s.Destroy()
	s.s = nil
	slog.Info("model unloaded while idle", "model", filepath.Base(s.path), "idle", d)
}

// loaded reports whether the session currently holds its weights.
func (s *idleSession) loaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.s != nil
}

// Destroy frees the session for good; later Runs fail.
func (s *idleSession) Destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
	if s.s == nil {
		return nil
	}
	err := s.s.Destroy()
	s.s = nil
	return err
}
