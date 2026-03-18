package protocol

import (
	"testing"
)

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		def     ErrorDef
		code    int
		message string
	}{
		{ErrDeviceNotConnected, -32001, "Device not connected"},
		{ErrDeviceNotFound, -32002, "Device not found"},
		{ErrWDAUnavailable, -32003, "WDA unavailable"},
		{ErrWDANotInstalled, -32004, "WDA not installed"},
		{ErrOperationTimeout, -32005, "Operation timeout"},
		{ErrAppNotFound, -32006, "App not found"},
		{ErrTunnelFailed, -32007, "Tunnel setup failed"},
		{ErrInvalidRequest, -32600, "Invalid request"},
		{ErrMethodNotFound, -32601, "Method not found"},
	}

	for _, tc := range tests {
		if tc.def.Code != tc.code {
			t.Errorf("%s Code: got %d, want %d", tc.message, tc.def.Code, tc.code)
		}
		if tc.def.Message != tc.message {
			t.Errorf("Code %d Message: got %q, want %q", tc.code, tc.def.Message, tc.message)
		}
	}
}

func TestErrorDefToError(t *testing.T) {
	err := ErrDeviceNotConnected.ToError(nil)
	if err == nil {
		t.Fatal("ToError returned nil")
	}
	if err.Code != -32001 {
		t.Errorf("Code: got %d, want -32001", err.Code)
	}
	if err.Message != "Device not connected" {
		t.Errorf("Message: got %q", err.Message)
	}
	if err.Data != nil {
		t.Errorf("Data: expected nil, got %v", err.Data)
	}
}

func TestErrorDefToErrorWithData(t *testing.T) {
	data := map[string]string{"detail": "connection refused"}
	err := ErrWDAUnavailable.ToError(data)
	if err.Data == nil {
		t.Fatal("Data should not be nil")
	}
	m, ok := err.Data.(map[string]string)
	if !ok {
		t.Fatalf("Data has unexpected type %T", err.Data)
	}
	if m["detail"] != "connection refused" {
		t.Errorf("Data detail: got %q", m["detail"])
	}
}

func TestAllErrorCodesUnique(t *testing.T) {
	all := []ErrorDef{
		ErrDeviceNotConnected,
		ErrDeviceNotFound,
		ErrWDAUnavailable,
		ErrWDANotInstalled,
		ErrOperationTimeout,
		ErrAppNotFound,
		ErrTunnelFailed,
		ErrInvalidRequest,
		ErrMethodNotFound,
	}
	seen := map[int]bool{}
	for _, e := range all {
		if seen[e.Code] {
			t.Errorf("Duplicate error code: %d", e.Code)
		}
		seen[e.Code] = true
	}
}
