package wda

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ios-pilot/internal/driver"
)

// Compile-time interface check.
var _ driver.WDADriver = (*WDAClient)(nil)

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

func TestStatus_Ready(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/status" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{"ready": true},
		})
	}))
	defer srv.Close()

	c := NewWDAClient()
	ready, err := c.Status(srv.URL)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !ready {
		t.Error("expected ready=true")
	}
}

func TestStatus_NotReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{"ready": false},
		})
	}))
	defer srv.Close()

	c := NewWDAClient()
	ready, err := c.Status(srv.URL)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if ready {
		t.Error("expected ready=false")
	}
}

// ---------------------------------------------------------------------------
// CreateSession
// ---------------------------------------------------------------------------

func TestCreateSession(t *testing.T) {
	const wantID = "abc-123-def"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/session" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		// Verify content-type and body contain empty capabilities.
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("invalid JSON body: %v", err)
		}
		if _, ok := req["capabilities"]; !ok {
			t.Error("expected 'capabilities' field in body")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{"sessionId": wantID},
		})
	}))
	defer srv.Close()

	c := NewWDAClient()
	id, err := c.CreateSession(srv.URL)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if id != wantID {
		t.Errorf("session ID: got %q, want %q", id, wantID)
	}
}

// ---------------------------------------------------------------------------
// DeleteSession
// ---------------------------------------------------------------------------

func TestDeleteSession(t *testing.T) {
	const sessionID = "sess-xyz"
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewWDAClient()
	if err := c.DeleteSession(srv.URL, sessionID); err != nil {
		t.Fatalf("DeleteSession error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method: got %q, want DELETE", gotMethod)
	}
	if gotPath != "/session/"+sessionID {
		t.Errorf("path: got %q, want /session/%s", gotPath, sessionID)
	}
}

// ---------------------------------------------------------------------------
// Tap
// ---------------------------------------------------------------------------

func TestTap_W3CActions(t *testing.T) {
	const sessionID = "tap-session"
	var body []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/"+sessionID+"/actions" {
			body, _ = io.ReadAll(r.Body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewWDAClient()
	if err := c.Tap(srv.URL, sessionID, 100, 200); err != nil {
		t.Fatalf("Tap error: %v", err)
	}

	// Verify W3C Actions structure.
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid actions JSON: %v", err)
	}

	actions, ok := payload["actions"].([]interface{})
	if !ok || len(actions) == 0 {
		t.Fatal("expected non-empty 'actions' array")
	}

	action := actions[0].(map[string]interface{})
	if action["type"] != "pointer" {
		t.Errorf("action type: got %v, want 'pointer'", action["type"])
	}

	params := action["parameters"].(map[string]interface{})
	if params["pointerType"] != "touch" {
		t.Errorf("pointerType: got %v, want 'touch'", params["pointerType"])
	}

	steps := action["actions"].([]interface{})
	// Expect: pointerMove, pointerDown, pause, pointerUp
	if len(steps) != 4 {
		t.Fatalf("steps count: got %d, want 4", len(steps))
	}

	move := steps[0].(map[string]interface{})
	if move["type"] != "pointerMove" {
		t.Errorf("step[0] type: got %v, want 'pointerMove'", move["type"])
	}
	if x := move["x"].(float64); x != 100 {
		t.Errorf("step[0].x: got %v, want 100", x)
	}
	if y := move["y"].(float64); y != 200 {
		t.Errorf("step[0].y: got %v, want 200", y)
	}

	down := steps[1].(map[string]interface{})
	if down["type"] != "pointerDown" {
		t.Errorf("step[1] type: got %v, want 'pointerDown'", down["type"])
	}

	pause := steps[2].(map[string]interface{})
	if pause["type"] != "pause" {
		t.Errorf("step[2] type: got %v, want 'pause'", pause["type"])
	}

	up := steps[3].(map[string]interface{})
	if up["type"] != "pointerUp" {
		t.Errorf("step[3] type: got %v, want 'pointerUp'", up["type"])
	}
}

// ---------------------------------------------------------------------------
// Swipe
// ---------------------------------------------------------------------------

func TestSwipe_W3CActions(t *testing.T) {
	const sessionID = "swipe-session"
	var body []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/"+sessionID+"/actions" {
			body, _ = io.ReadAll(r.Body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewWDAClient()
	if err := c.Swipe(srv.URL, sessionID, 10, 20, 300, 400); err != nil {
		t.Fatalf("Swipe error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	actions := payload["actions"].([]interface{})
	action := actions[0].(map[string]interface{})
	steps := action["actions"].([]interface{})

	// Expect: pointerMove(start), pointerDown, pointerMove(end), pointerUp
	if len(steps) != 4 {
		t.Fatalf("steps count: got %d, want 4", len(steps))
	}

	startMove := steps[0].(map[string]interface{})
	if startMove["x"].(float64) != 10 || startMove["y"].(float64) != 20 {
		t.Errorf("start coords: got (%v,%v), want (10,20)", startMove["x"], startMove["y"])
	}

	endMove := steps[2].(map[string]interface{})
	if endMove["type"] != "pointerMove" {
		t.Errorf("step[2] type: got %v, want 'pointerMove'", endMove["type"])
	}
	if endMove["x"].(float64) != 300 || endMove["y"].(float64) != 400 {
		t.Errorf("end coords: got (%v,%v), want (300,400)", endMove["x"], endMove["y"])
	}
	// Verify move duration is set for the swipe (non-zero).
	if dur := endMove["duration"].(float64); dur <= 0 {
		t.Errorf("swipe move duration: got %v, want > 0", dur)
	}
}

// ---------------------------------------------------------------------------
// InputText
// ---------------------------------------------------------------------------

func TestInputText_CharsSplit(t *testing.T) {
	const sessionID = "text-session"
	var body []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/"+sessionID+"/wda/keys" {
			body, _ = io.ReadAll(r.Body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewWDAClient()
	if err := c.InputText(srv.URL, sessionID, "Hi!"); err != nil {
		t.Fatalf("InputText error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	chars, ok := payload["value"].([]interface{})
	if !ok {
		t.Fatal("expected 'value' to be an array")
	}
	want := []string{"H", "i", "!"}
	if len(chars) != len(want) {
		t.Fatalf("chars count: got %d, want %d", len(chars), len(want))
	}
	for i, ch := range want {
		if chars[i].(string) != ch {
			t.Errorf("chars[%d]: got %q, want %q", i, chars[i], ch)
		}
	}
}

func TestInputText_Unicode(t *testing.T) {
	const sessionID = "unicode-session"
	var body []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewWDAClient()
	// "你好" — 2 Unicode code points, should produce 2 chars.
	if err := c.InputText(srv.URL, sessionID, "你好"); err != nil {
		t.Fatalf("InputText error: %v", err)
	}

	var payload map[string]interface{}
	json.Unmarshal(body, &payload)
	chars := payload["value"].([]interface{})
	if len(chars) != 2 {
		t.Errorf("unicode chars: got %d, want 2", len(chars))
	}
}

// ---------------------------------------------------------------------------
// Screenshot
// ---------------------------------------------------------------------------

func TestScreenshot_Base64Decode(t *testing.T) {
	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG magic
	encoded := base64.StdEncoding.EncodeToString(pngBytes)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/screenshot" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": encoded,
		})
	}))
	defer srv.Close()

	c := NewWDAClient()
	data, err := c.Screenshot(srv.URL, "ignored-session")
	if err != nil {
		t.Fatalf("Screenshot error: %v", err)
	}
	if len(data) != len(pngBytes) {
		t.Fatalf("data length: got %d, want %d", len(data), len(pngBytes))
	}
	for i, b := range pngBytes {
		if data[i] != b {
			t.Errorf("data[%d]: got 0x%02x, want 0x%02x", i, data[i], b)
		}
	}
}
