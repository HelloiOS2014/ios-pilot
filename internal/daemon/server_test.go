package daemon

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ios-pilot/internal/protocol"
)

// startTestServer starts the server and waits briefly for it to be ready.
func startTestServer(t *testing.T, sockPath string) *Server {
	t.Helper()
	srv := NewServer(sockPath)
	if err := srv.Start(); err != nil {
		t.Fatalf("Server.Start: %v", err)
	}
	// Give the goroutine time to begin listening.
	time.Sleep(10 * time.Millisecond)
	return srv
}

// dialAndRoundTrip sends one JSON-RPC request line and reads one response line.
func dialAndRoundTrip(t *testing.T, sockPath string, req *protocol.Request) *protocol.Response {
	t.Helper()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	if err := enc.Encode(req); err != nil {
		t.Fatalf("encode request: %v", err)
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatalf("no response received: %v", scanner.Err())
	}

	var resp protocol.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return &resp
}

func TestServerStartStop(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	srv := startTestServer(t, sockPath)

	srv.Handle("ping", func(params json.RawMessage) (any, error) {
		return map[string]string{"pong": "ok"}, nil
	})

	req := protocol.NewRequest(1, "ping", nil)
	resp := dialAndRoundTrip(t, sockPath, req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected result, got nil")
	}

	srv.Stop()
}

func TestServerMethodNotFound(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	srv := startTestServer(t, sockPath)
	defer srv.Stop()

	req := protocol.NewRequest(2, "nonexistent.method", nil)
	resp := dialAndRoundTrip(t, sockPath, req)

	if resp.Error == nil {
		t.Fatal("expected error response, got nil error")
	}
	if resp.Error.Code != protocol.ErrMethodNotFound.Code {
		t.Errorf("error code: got %d, want %d", resp.Error.Code, protocol.ErrMethodNotFound.Code)
	}
}

func TestSocketCleanupOnStop(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "cleanup.sock")
	srv := startTestServer(t, sockPath)

	// Socket file should exist while server is running.
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		t.Fatal("socket file should exist while server is running")
	}

	srv.Stop()

	// Socket file should be removed after Stop.
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Error("socket file should be removed after Stop()")
	}
}

func TestServerHandlerError(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	srv := startTestServer(t, sockPath)
	defer srv.Stop()

	// Handler that returns a protocol.Error.
	srv.Handle("fail.proto", func(params json.RawMessage) (any, error) {
		return nil, protocol.ErrDeviceNotConnected.ToError(nil)
	})

	// Handler that returns a plain error.
	srv.Handle("fail.plain", func(params json.RawMessage) (any, error) {
		return nil, os.ErrNotExist
	})

	// Test protocol error forwarding.
	resp := dialAndRoundTrip(t, sockPath, protocol.NewRequest(3, "fail.proto", nil))
	if resp.Error == nil {
		t.Fatal("expected error for fail.proto")
	}
	if resp.Error.Code != protocol.ErrDeviceNotConnected.Code {
		t.Errorf("error code: got %d, want %d", resp.Error.Code, protocol.ErrDeviceNotConnected.Code)
	}

	// Test plain error wrapping as -32603 internal error.
	resp2 := dialAndRoundTrip(t, sockPath, protocol.NewRequest(4, "fail.plain", nil))
	if resp2.Error == nil {
		t.Fatal("expected error for fail.plain")
	}
	if resp2.Error.Code != -32603 {
		t.Errorf("error code: got %d, want -32603", resp2.Error.Code)
	}
}

func TestServerMultipleRequests(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "multi.sock")
	srv := startTestServer(t, sockPath)
	defer srv.Stop()

	srv.Handle("echo", func(params json.RawMessage) (any, error) {
		return string(params), nil
	})

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	scanner := bufio.NewScanner(conn)

	for i := 1; i <= 3; i++ {
		params, _ := json.Marshal(map[string]int{"seq": i})
		req := protocol.NewRequest(i, "echo", params)
		if err := enc.Encode(req); err != nil {
			t.Fatalf("encode req %d: %v", i, err)
		}
		if !scanner.Scan() {
			t.Fatalf("no response for req %d", i)
		}
		var resp protocol.Response
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			t.Fatalf("decode resp %d: %v", i, err)
		}
		if resp.Error != nil {
			t.Fatalf("unexpected error for req %d: %+v", i, resp.Error)
		}
	}
}

func TestServerIdleTimerReset(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "idle.sock")
	srv := startTestServer(t, sockPath)
	defer srv.Stop()

	fired := make(chan struct{}, 1)
	it := NewIdleTimer(200*time.Millisecond, func() {
		fired <- struct{}{}
	})
	srv.SetIdleTimer(it)

	srv.Handle("ping", func(params json.RawMessage) (any, error) {
		return "pong", nil
	})

	// Send a request — this should reset the timer.
	dialAndRoundTrip(t, sockPath, protocol.NewRequest(1, "ping", nil))

	// The timer should NOT have fired yet (we just reset it).
	select {
	case <-fired:
		t.Fatal("idle timer fired immediately after request")
	case <-time.After(50 * time.Millisecond):
		// Good.
	}
}
