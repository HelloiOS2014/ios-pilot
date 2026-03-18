// Package client provides a JSON-RPC 2.0 client over a Unix domain socket
// for communicating with the ios-pilot daemon.
package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"ios-pilot/internal/protocol"
)

const defaultTimeout = 30 * time.Second

// Client is a JSON-RPC 2.0 client connected to the ios-pilot daemon via a
// Unix domain socket.
type Client struct {
	conn    net.Conn
	scanner *bufio.Scanner
	enc     *json.Encoder
	nextID  atomic.Int64
	timeout time.Duration
}

// Dial connects to the Unix socket at sockPath and returns a ready Client.
func Dial(sockPath string) (*Client, error) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("dial unix %s: %w", sockPath, err)
	}
	return &Client{
		conn:    conn,
		scanner: bufio.NewScanner(conn),
		enc:     json.NewEncoder(conn),
		timeout: defaultTimeout,
	}, nil
}

// Call sends a JSON-RPC 2.0 request with the given method and params,
// blocks until a response is received (or the read deadline is exceeded),
// and returns the response.
//
// params may be any JSON-serialisable value or nil.
func (c *Client) Call(method string, params any) (*protocol.Response, error) {
	id := c.nextID.Add(1)

	var rawParams json.RawMessage
	if params != nil {
		var err error
		rawParams, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
	}

	req := protocol.NewRequest(id, method, rawParams)
	if err := c.enc.Encode(req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Set a deadline so we never block forever.
	if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	if !c.scanner.Scan() {
		if err := c.scanner.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, fmt.Errorf("connection closed before response")
	}

	// Clear deadline.
	c.conn.SetReadDeadline(time.Time{}) //nolint:errcheck

	var resp protocol.Response
	if err := json.Unmarshal(c.scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}

// Close closes the underlying connection.
func (c *Client) Close() {
	c.conn.Close()
}
