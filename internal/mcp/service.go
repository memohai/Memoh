package mcp

import (
	"encoding/json"
	"errors"
	"strconv"
)

// JSONRPCRequest is the JSON-RPC 2.0 request shape (jsonrpc, id, method, params).
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is the JSON-RPC 2.0 response shape (result or error).
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is the JSON-RPC 2.0 error object (code, message).
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewToolCallRequest builds a tools/call JSON-RPC request with the given id, tool name, and arguments.
func NewToolCallRequest(id, toolName string, args map[string]any) (JSONRPCRequest, error) {
	params := map[string]any{
		"name":      toolName,
		"arguments": args,
	}
	rawParams, err := json.Marshal(params)
	if err != nil {
		return JSONRPCRequest{}, err
	}
	return JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      RawStringID(id),
		Method:  "tools/call",
		Params:  rawParams,
	}, nil
}

// RawStringID returns a JSON-RPC id as quoted string raw message.
func RawStringID(id string) json.RawMessage {
	return json.RawMessage([]byte(strconv.Quote(id)))
}

// PayloadError returns an error if the payload contains a top-level error object.
func PayloadError(payload map[string]any) error {
	if payload == nil {
		return errors.New("empty payload")
	}
	if errObj, ok := payload["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok && msg != "" {
			return errors.New(msg)
		}
		return errors.New("mcp error")
	}
	return nil
}

// ResultError returns an error if result.isError is true (tool call failure).
func ResultError(payload map[string]any) error {
	result, ok := payload["result"].(map[string]any)
	if !ok {
		return nil
	}
	if isErr, ok := result["isError"].(bool); ok && isErr {
		msg := ContentText(result)
		if msg == "" {
			msg = "mcp tool error"
		}
		return errors.New(msg)
	}
	return nil
}

// StructuredContent extracts result.structuredContent from the payload, or parses result.content text as JSON.
func StructuredContent(payload map[string]any) (map[string]any, error) {
	result, ok := payload["result"].(map[string]any)
	if !ok {
		return nil, errors.New("missing result")
	}
	if structured, ok := result["structuredContent"].(map[string]any); ok {
		return structured, nil
	}
	if content := ContentText(result); content != "" {
		var out map[string]any
		if err := json.Unmarshal([]byte(content), &out); err == nil {
			return out, nil
		}
	}
	return nil, errors.New("missing structured content")
}

// ContentText returns the first content item's text from the MCP result content array.
func ContentText(result map[string]any) string {
	rawContent, ok := result["content"].([]any)
	if !ok || len(rawContent) == 0 {
		return ""
	}
	first, ok := rawContent[0].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := first["text"].(string)
	return text
}
