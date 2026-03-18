package protocol

// ErrorDef represents a JSON-RPC error definition with a fixed code and message.
type ErrorDef struct {
	Code    int
	Message string
}

// ToError converts an ErrorDef to an Error, optionally attaching additional data.
func (e ErrorDef) ToError(data any) *Error {
	return &Error{
		Code:    e.Code,
		Message: e.Message,
		Data:    data,
	}
}

// Application-specific error codes (server-defined range: -32099 to -32000)
// and JSON-RPC predefined errors.
var (
	// Application errors
	ErrDeviceNotConnected = ErrorDef{-32001, "Device not connected"}
	ErrDeviceNotFound     = ErrorDef{-32002, "Device not found"}
	ErrWDAUnavailable     = ErrorDef{-32003, "WDA unavailable"}
	ErrWDANotInstalled    = ErrorDef{-32004, "WDA not installed"}
	ErrOperationTimeout   = ErrorDef{-32005, "Operation timeout"}
	ErrAppNotFound        = ErrorDef{-32006, "App not found"}
	ErrTunnelFailed       = ErrorDef{-32007, "Tunnel setup failed"}

	// JSON-RPC predefined errors
	ErrInvalidRequest = ErrorDef{-32600, "Invalid request"}
	ErrMethodNotFound = ErrorDef{-32601, "Method not found"}
)
