package mcp

import (
	"encoding/json"
)

// Error Codes
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// JSON-RPC 2.0 Error Object
// Used in a response when there's a protocol error
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

func NewParseError(msg string) *Error {
	return &Error{Code: CodeParseError, Message: msg}
}

func NewInvalidRequest(msg string) *Error {
	return &Error{Code: CodeInvalidRequest, Message: msg}
}

func NewMethodNotFound(method string) *Error {
	return &Error{Code: CodeMethodNotFound, Message: "method not found: " + method}
}

func NewInvalidParams(msg string) *Error {
	return &Error{Code: CodeInvalidParams, Message: msg}
}

func NewInternalError(msg string) *Error {
	return &Error{Code: CodeInternalError, Message: msg}
}
