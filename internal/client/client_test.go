package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ios-pilot/internal/daemon"
	"ios-pilot/internal/protocol"
)

// shortTempDir creates a temp directory under /tmp to keep Unix socket paths
// short enough for macOS (104-byte limit on sun_path).
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ip_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// startEchoServer starts a test daemon.Server with an "echo" handler and a
// "fail" handler, returns the server and its socket path.
func startEchoServer(t *testing.T) (*daemon.Server, string) {
	t.Helper()
	sockPath := filepath.Join(shortTempDir(t), "t.sock")
	srv := daemon.NewServer(sockPath)

	srv.Handle("echo", func(params json.RawMessage) (any, error) {
		var m map[string]any
		if err := json.Unmarshal(params, &m); err != nil {
			return nil, err
		}
		return m, nil
	})

	srv.Handle("fail", func(params json.RawMessage) (any, error) {
		return nil, protocol.ErrDeviceNotConnected.ToError("test error")
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("server Start: %v", err)
	}
	// Give goroutine time to start listening.
	time.Sleep(10 * time.Millisecond)

	t.Cleanup(srv.Stop)
	return srv, sockPath
}

func TestClientRoundTrip(t *testing.T) {
	_, sockPath := startEchoServer(t)

	c, err := Dial(sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	params := map[string]string{"hello": "world"}
	resp, err := c.Call("echo", params)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestClientErrorResponse(t *testing.T) {
	_, sockPath := startEchoServer(t)

	c, err := Dial(sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	resp, err := c.Call("fail", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error in response, got nil")
	}
	if resp.Error.Code != protocol.ErrDeviceNotConnected.Code {
		t.Errorf("error code: got %d, want %d", resp.Error.Code, protocol.ErrDeviceNotConnected.Code)
	}
}

func TestClientMethodNotFound(t *testing.T) {
	_, sockPath := startEchoServer(t)

	c, err := Dial(sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	resp, err := c.Call("nonexistent", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error in response for unknown method")
	}
	if resp.Error.Code != protocol.ErrMethodNotFound.Code {
		t.Errorf("error code: got %d, want %d", resp.Error.Code, protocol.ErrMethodNotFound.Code)
	}
}

func TestClientIncrementingID(t *testing.T) {
	_, sockPath := startEchoServer(t)

	c, err := Dial(sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	params := map[string]string{"x": "1"}

	resp1, err := c.Call("echo", params)
	if err != nil {
		t.Fatalf("Call 1: %v", err)
	}
	resp2, err := c.Call("echo", params)
	if err != nil {
		t.Fatalf("Call 2: %v", err)
	}

	// IDs should be non-nil and different.
	if resp1.ID == nil || resp2.ID == nil {
		t.Fatal("response IDs should not be nil")
	}
	if resp1.ID == resp2.ID {
		t.Error("consecutive calls should have different IDs")
	}
}

func TestClientDialMissingSock(t *testing.T) {
	_, err := Dial(filepath.Join(t.TempDir(), "missing.sock"))
	if err == nil {
		t.Fatal("expected error dialing non-existent socket")
	}
}
