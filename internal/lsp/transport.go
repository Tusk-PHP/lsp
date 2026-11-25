package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
)

type jsonRPCMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

func (s *Server) decodeParams[P any](msg *jsonRPCMessage) (P, bool) {
	var params P
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.rejectParams(msg, err)
		return params, false
	}
	return params, true
}

func (s *Server) rejectParams(msg *jsonRPCMessage, err error) {
	message := fmt.Sprintf("Invalid params for %s: %v", msg.Method, err)
	s.logger.Print(message)
	s.sendError(msg.ID, codeInvalidParams, message)
}

func (s *Server) readMessage() (*jsonRPCMessage, error) {
	var contentLength int
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			lengthStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(lengthStr)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %v", err)
			}
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, body); err != nil {
		return nil, err
	}
	var msg jsonRPCMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	s.logger.Printf("← %s", msg.Method)
	return &msg, nil
}

func (s *Server) sendResponse(id *json.RawMessage, result interface{}) {
	if id == nil {
		return
	}
	s.writeMessage(struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      interface{} `json:"id"`
		Result  interface{} `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) sendError(id *json.RawMessage, code int, message string) {
	if id == nil {
		return
	}
	s.writeMessage(jsonRPCResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func (s *Server) sendNotification(method string, params interface{}) {
	s.writeMessage(struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{JSONRPC: "2.0", Method: method, Params: params})
}

// sendServerRequest sends a server-to-client JSON-RPC request. The response
// is intentionally ignored ("fire-and-forget"). Use this for one-way side
// effects such as window/showDocument where the server has no business
// logic dependent on the client's reply.
//
// Each call generates a unique numeric ID. The client's response (if any)
// arrives back through the normal message loop and is silently dropped.
func (s *Server) sendServerRequest(method string, params interface{}) {
	id := atomic.AddInt64(&s.requestIDCounter, 1)
	s.writeMessage(struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      int64       `json:"id"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{JSONRPC: "2.0", ID: id, Method: method, Params: params})
}
