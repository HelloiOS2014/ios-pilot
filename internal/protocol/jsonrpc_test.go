package protocol

import (
	"encoding/json"
	"testing"
)

func TestNewRequest(t *testing.T) {
	params := map[string]string{"device_id": "abc123"}
	paramsJSON, _ := json.Marshal(params)
	req := NewRequest(1, "ios.connect", paramsJSON)

	if req.JSONRPC != "2.0" {
		t.Errorf("JSONRPC: got %q, want %q", req.JSONRPC, "2.0")
	}
	if req.ID != 1 {
		t.Errorf("ID: got %v, want 1", req.ID)
	}
	if req.Method != "ios.connect" {
		t.Errorf("Method: got %q, want %q", req.Method, "ios.connect")
	}
}

func TestNewResponse(t *testing.T) {
	result := map[string]bool{"connected": true}
	resp := NewResponse(42, result)

	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC: got %q, want %q", resp.JSONRPC, "2.0")
	}
	if resp.ID != 42 {
		t.Errorf("ID: got %v, want 42", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("Error should be nil for success response")
	}
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse(7, ErrDeviceNotConnected, nil)

	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC: got %q, want %q", resp.JSONRPC, "2.0")
	}
	if resp.ID != 7 {
		t.Errorf("ID: got %v, want 7", resp.ID)
	}
	if resp.Result != nil {
		t.Errorf("Result should be nil for error response")
	}
	if resp.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if resp.Error.Code != ErrDeviceNotConnected.Code {
		t.Errorf("Error.Code: got %d, want %d", resp.Error.Code, ErrDeviceNotConnected.Code)
	}
	if resp.Error.Message != ErrDeviceNotConnected.Message {
		t.Errorf("Error.Message: got %q, want %q", resp.Error.Message, ErrDeviceNotConnected.Message)
	}
}

func TestNewErrorResponseWithData(t *testing.T) {
	data := "additional detail"
	resp := NewErrorResponse(1, ErrWDAUnavailable, data)

	if resp.Error.Data != data {
		t.Errorf("Error.Data: got %v, want %v", resp.Error.Data, data)
	}
}

func TestNewNotification(t *testing.T) {
	params := map[string]string{"event": "device_connected"}
	notif := NewNotification("ios.event", params)

	if notif.JSONRPC != "2.0" {
		t.Errorf("JSONRPC: got %q, want %q", notif.JSONRPC, "2.0")
	}
	if notif.Method != "ios.event" {
		t.Errorf("Method: got %q, want %q", notif.Method, "ios.event")
	}
}

func TestDecodeRequest(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":5,"method":"ios.screenshot","params":{"device_id":"xyz"}}`
	req, err := DecodeRequest([]byte(raw))
	if err != nil {
		t.Fatalf("DecodeRequest error: %v", err)
	}
	if req.Method != "ios.screenshot" {
		t.Errorf("Method: got %q, want %q", req.Method, "ios.screenshot")
	}
	// ID is decoded as float64 from JSON by default for `any` type
	// Accept either float64(5) or int(5)
	switch v := req.ID.(type) {
	case float64:
		if v != 5 {
			t.Errorf("ID: got %v, want 5", req.ID)
		}
	case int:
		if v != 5 {
			t.Errorf("ID: got %v, want 5", req.ID)
		}
	default:
		t.Errorf("ID has unexpected type %T: %v", req.ID, req.ID)
	}
}

func TestDecodeRequestInvalid(t *testing.T) {
	_, err := DecodeRequest([]byte("not json"))
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestResponseSerialization(t *testing.T) {
	resp := NewErrorResponse("req-1", ErrMethodNotFound, nil)
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if m["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc field: got %v", m["jsonrpc"])
	}
	if _, ok := m["result"]; ok {
		t.Error("result field should be omitted for error response")
	}
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error field missing or wrong type")
	}
	if errObj["code"] != float64(ErrMethodNotFound.Code) {
		t.Errorf("error.code: got %v, want %d", errObj["code"], ErrMethodNotFound.Code)
	}
}

func TestNotificationNoID(t *testing.T) {
	notif := NewNotification("ios.log", nil)
	data, err := json.Marshal(notif)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if _, ok := m["id"]; ok {
		t.Error("Notification must not have id field")
	}
}
