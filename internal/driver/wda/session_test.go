package wda

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// EnsureSession
// ---------------------------------------------------------------------------

func TestEnsureSession_CreatesOnFirstCall(t *testing.T) {
	const wantID = "session-001"
	var createCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			atomic.AddInt32(&createCount, 1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"sessionId": wantID},
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	sm := NewSessionManager(srv.URL)
	id, err := sm.EnsureSession()
	if err != nil {
		t.Fatalf("EnsureSession error: %v", err)
	}
	if id != wantID {
		t.Errorf("session ID: got %q, want %q", id, wantID)
	}
	if createCount != 1 {
		t.Errorf("POST /session called %d times, want 1", createCount)
	}
}

func TestEnsureSession_ReusesExisting(t *testing.T) {
	const wantID = "session-002"
	var createCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/session" {
			atomic.AddInt32(&createCount, 1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"sessionId": wantID},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sm := NewSessionManager(srv.URL)

	// First call — creates session.
	id1, err := sm.EnsureSession()
	if err != nil {
		t.Fatalf("first EnsureSession error: %v", err)
	}

	// Second call — should reuse.
	id2, err := sm.EnsureSession()
	if err != nil {
		t.Fatalf("second EnsureSession error: %v", err)
	}

	if id1 != id2 {
		t.Errorf("session IDs differ: %q vs %q", id1, id2)
	}
	if createCount != 1 {
		t.Errorf("POST /session called %d times, want exactly 1", createCount)
	}
}

// ---------------------------------------------------------------------------
// IsHealthy
// ---------------------------------------------------------------------------

func TestIsHealthy_True(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{"ready": true},
		})
	}))
	defer srv.Close()

	sm := NewSessionManager(srv.URL)
	if !sm.IsHealthy() {
		t.Error("expected IsHealthy=true")
	}
}

func TestIsHealthy_False_ServerDown(t *testing.T) {
	// Use a closed server to simulate WDA being unreachable.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // close immediately

	sm := NewSessionManager(srv.URL)
	if sm.IsHealthy() {
		t.Error("expected IsHealthy=false when server is down")
	}
}

// ---------------------------------------------------------------------------
// Destroy
// ---------------------------------------------------------------------------

func TestDestroy_DeletesSession(t *testing.T) {
	const wantID = "to-delete"
	var deletedID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"sessionId": wantID},
			})
		case r.Method == http.MethodDelete:
			// e.g. /session/to-delete
			deletedID = r.URL.Path[len("/session/"):]
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	sm := NewSessionManager(srv.URL)
	if _, err := sm.EnsureSession(); err != nil {
		t.Fatalf("EnsureSession error: %v", err)
	}
	if err := sm.Destroy(); err != nil {
		t.Fatalf("Destroy error: %v", err)
	}
	if deletedID != wantID {
		t.Errorf("deleted session ID: got %q, want %q", deletedID, wantID)
	}
	// After destroy the cached ID should be cleared.
	if sm.SessionID() != "" {
		t.Errorf("SessionID after Destroy: got %q, want empty", sm.SessionID())
	}
}

func TestDestroy_NoOp_WhenNoSession(t *testing.T) {
	// Destroy before any session is created should not error.
	sm := NewSessionManager("http://127.0.0.1:9999") // unreachable, but not called
	if err := sm.Destroy(); err != nil {
		t.Errorf("Destroy with no session should be no-op, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SessionID
// ---------------------------------------------------------------------------

func TestSessionID_EmptyBeforeCreate(t *testing.T) {
	sm := NewSessionManager("http://127.0.0.1:9999")
	if id := sm.SessionID(); id != "" {
		t.Errorf("SessionID before create: got %q, want empty", id)
	}
}
