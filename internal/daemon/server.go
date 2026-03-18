package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"ios-pilot/internal/protocol"
)

// HandlerFunc is the signature for JSON-RPC method handlers.
// params is the raw JSON params field from the request (may be nil).
// Return (result, nil) for success; return a *protocol.Error or any other
// error for failure.
type HandlerFunc func(params json.RawMessage) (any, error)

// Server is a JSON-RPC 2.0 server over a Unix domain socket.
// Requests and responses are newline-delimited JSON.
type Server struct {
	sockPath string
	handlers map[string]HandlerFunc

	mu       sync.Mutex
	listener net.Listener

	wg        sync.WaitGroup
	idleTimer *IdleTimer
}

// NewServer creates a new Server that will listen on sockPath.
func NewServer(sockPath string) *Server {
	return &Server{
		sockPath: sockPath,
		handlers: make(map[string]HandlerFunc),
	}
}

// Handle registers handler for the given method name.
func (s *Server) Handle(method string, handler HandlerFunc) {
	s.handlers[method] = handler
}

// SetIdleTimer attaches an idle timer to the server.
// The timer is reset on every request.
func (s *Server) SetIdleTimer(it *IdleTimer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idleTimer = it
}

// Start begins listening on the Unix socket and accepting connections.
// Each connection is handled in its own goroutine.
// If a socket file already exists at sockPath it is removed first.
func (s *Server) Start() error {
	// Remove stale socket.
	os.Remove(s.sockPath)

	ln, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", s.sockPath, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	go s.acceptLoop(ln)
	return nil
}

// Stop closes the listener (causing acceptLoop to exit), waits for all
// active connections to finish, then removes the socket file.
func (s *Server) Stop() {
	s.mu.Lock()
	ln := s.listener
	s.mu.Unlock()

	if ln != nil {
		ln.Close()
	}
	s.wg.Wait()
	os.Remove(s.sockPath)
}

func (s *Server) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Listener closed — normal shutdown.
			return
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConn(c)
		}(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	enc := json.NewEncoder(conn)

	for scanner.Scan() {
		line := scanner.Bytes()

		// Reset idle timer on each request.
		s.mu.Lock()
		it := s.idleTimer
		s.mu.Unlock()
		if it != nil {
			it.Reset()
		}

		resp := s.dispatch(line)
		if err := enc.Encode(resp); err != nil {
			return
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		_ = err // ignore read errors on connection close
	}
}

// dispatch parses one JSON line and routes it to the registered handler.
func (s *Server) dispatch(line []byte) *protocol.Response {
	req, err := protocol.DecodeRequest(line)
	if err != nil {
		return protocol.NewErrorResponse(nil, protocol.ErrInvalidRequest, err.Error())
	}

	handler, ok := s.handlers[req.Method]
	if !ok {
		return protocol.NewErrorResponse(req.ID, protocol.ErrMethodNotFound, nil)
	}

	result, herr := handler(req.Params)
	if herr != nil {
		if protoErr, ok := herr.(*protocol.Error); ok {
			return &protocol.Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   protoErr,
			}
		}
		// Wrap as internal error (-32603).
		return &protocol.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &protocol.Error{
				Code:    -32603,
				Message: "Internal error",
				Data:    herr.Error(),
			},
		}
	}

	return protocol.NewResponse(req.ID, result)
}
