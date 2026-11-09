package lsp

import (
	"encoding/json"
	"strconv"
)

// writeMessage sends one JSON-RPC message to the client. The message loop and
// background goroutines (indexing, PHPStan and Pint on save, coverage) all
// send, so each frame (header and body) goes out as one write under writeMu:
// two frames never interleave on the stream.
func (s *Server) writeMessage(msg interface{}) {
	body, err := json.Marshal(msg)
	if err != nil {
		s.logger.Printf("Marshal error: %v", err)
		return
	}
	frame := make([]byte, 0, len(body)+32)
	frame = append(frame, "Content-Length: "...)
	frame = strconv.AppendInt(frame, int64(len(body)), 10)
	frame = append(frame, "\r\n\r\n"...)
	frame = append(frame, body...)

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.writer.Write(frame); err != nil {
		s.logger.Printf("Write error: %v", err)
	}
}
