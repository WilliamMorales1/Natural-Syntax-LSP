package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestSmoke runs the real server binary over stdio with a real model. Off by default: set NSLS_SMOKE_MODEL to a .onnx path.
// NSLS_SMOKE_SERVER uses a prebuilt binary instead of building this checkout.
func TestSmoke(t *testing.T) {
	model := os.Getenv("NSLS_SMOKE_MODEL")
	if model == "" {
		t.Skip("set NSLS_SMOKE_MODEL to an exported .onnx file")
	}
	server := os.Getenv("NSLS_SMOKE_SERVER")
	if server == "" {
		server = filepath.Join(t.TempDir(), "nsls")
		if runtime.GOOS == "windows" {
			server += ".exe"
		}
		build := exec.Command("go", "build", "-o", server, "../../cmd/natural-syntax-ls")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, out)
		}
	}

	cmd := exec.Command(server, "-mode", "pos", "-model", model)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		in.Close()
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
		}
	}()

	type msg struct {
		ID     *json.RawMessage `json:"id"`
		Result json.RawMessage  `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	msgs := make(chan msg, 64)
	go func() {
		r := bufio.NewReader(out)
		for {
			n := 0
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					close(msgs)
					return
				}
				line = strings.TrimSpace(line)
				if line == "" {
					break
				}
				fmt.Sscanf(line, "Content-Length: %d", &n)
			}
			body := make([]byte, n)
			if _, err := io.ReadFull(r, body); err != nil {
				close(msgs)
				return
			}
			var m msg
			if json.Unmarshal(body, &m) == nil {
				msgs <- m
			}
		}
	}()
	send := func(body string) {
		t.Helper()
		if _, err := fmt.Fprintf(in, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
			t.Fatal(err)
		}
	}
	request := func(id int, method, params string) msg {
		t.Helper()
		send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, params))
		timeout := time.After(180 * time.Second)
		for {
			select {
			case m, ok := <-msgs:
				if !ok {
					t.Fatalf("server exited before answering %s", method)
				}
				if m.ID != nil && string(*m.ID) == strconv.Itoa(id) {
					return m
				}
			case <-timeout:
				t.Fatalf("no answer to %s", method)
			}
		}
	}

	if m := request(1, "initialize", `{"processId":null,"rootUri":null,"capabilities":{}}`); m.Error != nil {
		t.Fatalf("initialize: %s", m.Error.Message)
	}
	send(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	send(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///smoke.txt","languageId":"text","version":1,"text":"The quick brown fox jumps over the lazy dog."}}}`)

	var data []uint32
	deadline := time.Now().Add(180 * time.Second)
	for id := 2; len(data) == 0 && time.Now().Before(deadline); id++ {
		m := request(id, "textDocument/semanticTokens/full", `{"textDocument":{"uri":"file:///smoke.txt"}}`)
		if m.Error != nil {
			t.Fatalf("semanticTokens: %s", m.Error.Message)
		}
		var res struct{ Data []uint32 }
		json.Unmarshal(m.Result, &res)
		data = res.Data
		if len(data) == 0 {
			time.Sleep(time.Second)
		}
	}
	// Nine words, five ints each.
	if len(data) != 45 {
		t.Fatalf("semantic tokens: %d ints, want 45: %v", len(data), data)
	}
	request(1000, "shutdown", `null`)
	send(`{"jsonrpc":"2.0","method":"exit"}`)
}
