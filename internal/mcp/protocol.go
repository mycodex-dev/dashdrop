package mcp

import (
	"encoding/json"
	"fmt"
)

const (
	jsonRPCVersion   = "2.0"
	serverName       = "dashdrop"
	serverVersion    = "1.0.0"
	protocolVersion  = "2025-06-18"
	legacyProtocol   = "2025-03-26"
	oldestProtocol   = "2024-11-05"
	headerSessionID  = "Mcp-Session-Id"
	headerProtocol   = "Mcp-Protocol-Version"
)

// supportedProtocols lists versions this server can speak (newest first).
var supportedProtocols = []string{protocolVersion, legacyProtocol, oldestProtocol}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	errParse      = -32700
	errInvalidReq = -32600
	errMethod     = -32601
	errInvalidParams = -32602
	errInternal   = -32603
)

func successResponse(id json.RawMessage, result any) jsonRPCResponse {
	return jsonRPCResponse{JSONRPC: jsonRPCVersion, ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, message string, data any) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error:   &jsonRPCError{Code: code, Message: message, Data: data},
	}
}

type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type callToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func textResult(text string) toolResult {
	return toolResult{Content: []toolContent{{Type: "text", Text: text}}}
}

func errorResult(msg string) toolResult {
	return toolResult{
		Content: []toolContent{{Type: "text", Text: msg}},
		IsError: true,
	}
}

func negotiateProtocol(requested string) string {
	for _, v := range supportedProtocols {
		if v == requested {
			return v
		}
	}
	return protocolVersion
}

func formatToolJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
