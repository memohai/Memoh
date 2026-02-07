package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

func IsNotification(req JSONRPCRequest) bool {
	return len(req.ID) == 0 && strings.HasPrefix(req.Method, "notifications/")
}

func JSONRPCErrorResponse(id json.RawMessage, code int, message string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	}
}

func BuildPayloads(req JSONRPCRequest, initOnce *sync.Once) ([]string, json.RawMessage, error) {
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}
	targetID := req.ID
	payloads := []string{}
	shouldInit := req.Method != "initialize" && req.Method != "notifications/initialized"
	if initOnce != nil {
		ran := false
		initOnce.Do(func() {
			ran = true
		})
		if ran {
			// This is the first call on the session.
		} else {
			shouldInit = false
		}
	}
	if shouldInit {
		initReq := map[string]any{
			"jsonrpc": "2.0",
			"id":      "init-1",
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities": map[string]any{
					"roots": map[string]any{
						"listChanged": false,
					},
				},
				"clientInfo": map[string]any{
					"name":    "memoh-http-proxy",
					"version": "v0",
				},
			},
		}
		initBytes, err := json.Marshal(initReq)
		if err != nil {
			return nil, nil, err
		}
		payloads = append(payloads, string(initBytes))

		initialized := map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/initialized",
		}
		initializedBytes, err := json.Marshal(initialized)
		if err != nil {
			return nil, nil, err
		}
		payloads = append(payloads, string(initializedBytes))
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	payloads = append(payloads, string(reqBytes))
	return payloads, targetID, nil
}

func BuildNotificationPayloads(req JSONRPCRequest) ([]string, error) {
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}
	if strings.TrimSpace(req.Method) == "" {
		return nil, fmt.Errorf("missing method")
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return []string{string(reqBytes)}, nil
}
