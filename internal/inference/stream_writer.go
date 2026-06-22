/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"fmt"
	"net/http"
)

// StreamWriter writes SSE-formatted chunks for streaming inference.
type StreamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	closed  bool
}

// NewStreamWriter creates a streaming response writer.
// Sets headers for SSE (Server-Sent Events) and disables buffering.
func NewStreamWriter(w http.ResponseWriter) (*StreamWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("response writer does not support flushing")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	return &StreamWriter{w: w, flusher: flusher}, nil
}

// WriteChunk sends a single SSE data event and flushes immediately.
func (s *StreamWriter) WriteChunk(data []byte) error {
	if s.closed {
		return fmt.Errorf("stream closed")
	}
	_, err := fmt.Fprintf(s.w, "data: %s\n\n", data)
	if err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteDone sends the [DONE] sentinel and closes the stream.
func (s *StreamWriter) WriteDone() error {
	if s.closed {
		return nil
	}
	s.closed = true
	_, err := fmt.Fprint(s.w, "data: [DONE]\n\n")
	if err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}
