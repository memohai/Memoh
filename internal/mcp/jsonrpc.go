package mcp

import (
	"encoding/json"
	"strings"
)

// IsNotification reports whether the request is a notification (no id, method starts with notifications/).
func IsNotification(req JSONRPCRequest) bool {
	return len(req.ID) == 0 && strings.HasPrefix(req.Method, "notifications/")
}

// JSONRPCErrorResponse builds a JSON-RPC response with the given error code and message.
func JSONRPCErrorResponse(id json.RawMessage, code int, message string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	}
}
