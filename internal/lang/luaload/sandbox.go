// Package luaload turns Lua scripts into lang.Language values.
// Scripts run in a sandbox with no file, network, process or loader access; each call has a time limit and a failing hook only disables itself.
package luaload

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	lua "github.com/yuin/gopher-lua"
)

const (
	loadTimeout = 2 * time.Second
	callTimeout = 200 * time.Millisecond
	// maxRep caps string.rep, since a Lua heap cannot be limited from outside.
	maxRep = 1 << 16
)

// sandbox is one Lua state guarded by a mutex; hooks are pure, so the state is reusable.
type sandbox struct {
	name string
	mu   sync.Mutex
	L    *lua.LState
}

func newSandbox(name string) *sandbox {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	for _, lib := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(lib.fn))
		L.Push(lua.LString(lib.name))
		L.Call(1, 0)
	}
	for _, g := range []string{"dofile", "loadfile", "load", "loadstring", "require", "module", "print", "collectgarbage", "getfenv", "setfenv", "newproxy"} {
		L.SetGlobal(g, lua.LNil)
	}
	str := L.GetGlobal("string").(*lua.LTable)
	str.RawSetString("dump", lua.LNil)
	rep := str.RawGetString("rep")
	str.RawSetString("rep", L.NewFunction(func(L *lua.LState) int {
		if n := L.OptInt(2, 0); n*len(L.CheckString(1)) > maxRep {
			L.RaiseError("string.rep result too long")
		}
		L.Push(rep)
		L.Insert(rep, 1)
		L.Call(L.GetTop()-1, 1)
		return 1
	}))
	L.SetGlobal("utf8", utf8Lib(L))
	return &sandbox{name: name, L: L}
}

// utf8Lib is the small rune API scripts get, since Lua strings are bytes.
func utf8Lib(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	L.SetFuncs(t, map[string]lua.LGFunction{
		// chars returns the characters of a string as an array of strings.
		"chars": func(L *lua.LState) int {
			out := L.NewTable()
			for _, r := range L.CheckString(1) {
				out.Append(lua.LString(string(r)))
			}
			L.Push(out)
			return 1
		},
		// codepoint returns the code point of the first character.
		"codepoint": func(L *lua.LState) int {
			r, _ := utf8.DecodeRuneInString(L.CheckString(1))
			L.Push(lua.LNumber(r))
			return 1
		},
		"char": func(L *lua.LState) int {
			L.Push(lua.LString(string(rune(L.CheckInt(1)))))
			return 1
		},
		"len": func(L *lua.LState) int {
			L.Push(lua.LNumber(utf8.RuneCountInString(L.CheckString(1))))
			return 1
		},
	})
	return t
}

// run executes the chunk and returns its single result.
func (s *sandbox) run(src string) (lua.LValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
	defer cancel()
	s.L.SetContext(ctx)
	defer s.L.RemoveContext()
	fn, err := s.L.Load(strings.NewReader(src), s.name)
	if err != nil {
		return nil, err
	}
	s.L.Push(fn)
	if err := s.L.PCall(0, 1, nil); err != nil {
		return nil, err
	}
	defer s.L.Pop(1)
	return s.L.Get(-1), nil
}

// call runs a hook and returns its first result, or ok=false after logging a failure.
func (s *sandbox) call(fn *lua.LFunction, args ...lua.LValue) (res lua.LValue, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("lua hook panic", "script", s.name, "err", fmt.Sprint(r))
			res, ok = nil, false
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	s.L.SetContext(ctx)
	defer s.L.RemoveContext()
	top := s.L.GetTop()
	defer s.L.SetTop(top)
	s.L.Push(fn)
	for _, a := range args {
		s.L.Push(a)
	}
	if err := s.L.PCall(len(args), 1, nil); err != nil {
		slog.Warn("lua hook failed", "script", s.name, "err", err)
		return nil, false
	}
	return s.L.Get(-1), true
}
