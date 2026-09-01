package main

// client.go — a minimal, server-agnostic LSP client over stdio.
//
// It speaks nothing but the base protocol: Content-Length framing, JSON-RPC
// request/response correlation, notification fan-out, and automatic replies to
// the server->client requests that real editors answer. Anything specific to a
// particular language server lives in servers.json, never here.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message) }

type rpcMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

// Notification is a server->client notification with the time it was observed.
type Notification struct {
	Method string
	Params json.RawMessage
	At     time.Time
}

type diagEntry struct {
	Params json.RawMessage
	At     time.Time
}

// Client drives one language server process.
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan rpcMessage
	subs    map[int]chan Notification
	nextSub int
	notes   []Notification
	diags   map[string]diagEntry

	stderrMu sync.Mutex
	stderr   bytes.Buffer

	exited  chan struct{}
	exitErr error
}

// StartClient spawns the server and begins reading its stdout.
func StartClient(argv []string, dir string, env []string) (*Client, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		pending: make(map[int64]chan rpcMessage),
		subs:    make(map[int]chan Notification),
		diags:   make(map[string]diagEntry),
		exited:  make(chan struct{}),
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go c.drainStderr(stderr)
	go c.readLoop(stdout)
	go func() {
		c.exitErr = cmd.Wait()
		close(c.exited)
	}()
	return c, nil
}

func (c *Client) drainStderr(r io.Reader) {
	buf := make([]byte, 8192)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			c.stderrMu.Lock()
			// Keep the tail bounded; stderr is only used for failure diagnosis.
			if c.stderr.Len() < 1<<20 {
				c.stderr.Write(buf[:n])
			}
			c.stderrMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// Stderr returns whatever the server wrote to stderr so far.
func (c *Client) Stderr() string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	return c.stderr.String()
}

func (c *Client) readLoop(r io.Reader) {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		length, err := readHeader(br)
		if err != nil {
			c.failPending(fmt.Errorf("read header: %w", err))
			return
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(br, body); err != nil {
			c.failPending(fmt.Errorf("read body: %w", err))
			return
		}
		var msg rpcMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			continue // ignore junk rather than killing the run
		}
		c.dispatch(msg)
	}
}

func readHeader(br *bufio.Reader) (int, error) {
	length := -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if length < 0 {
				return 0, fmt.Errorf("missing Content-Length")
			}
			return length, nil
		}
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			key := strings.ToLower(strings.TrimSpace(line[:idx]))
			val := strings.TrimSpace(line[idx+1:])
			if key == "content-length" {
				n, err := strconv.Atoi(val)
				if err != nil {
					return 0, fmt.Errorf("bad Content-Length %q", val)
				}
				length = n
			}
		}
	}
}

func (c *Client) dispatch(msg rpcMessage) {
	switch {
	case msg.ID != nil && msg.Method != "":
		c.answerServerRequest(msg)
	case msg.ID != nil:
		var id int64
		if err := json.Unmarshal(*msg.ID, &id); err != nil {
			return
		}
		c.mu.Lock()
		ch, ok := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if ok {
			ch <- msg
		}
	case msg.Method != "":
		n := Notification{Method: msg.Method, Params: msg.Params, At: time.Now()}
		c.mu.Lock()
		c.notes = append(c.notes, n)
		if msg.Method == "textDocument/publishDiagnostics" {
			var p struct {
				URI string `json:"uri"`
			}
			if json.Unmarshal(msg.Params, &p) == nil {
				c.diags[p.URI] = diagEntry{Params: msg.Params, At: n.At}
			}
		}
		for _, ch := range c.subs {
			select {
			case ch <- n:
			default: // slow subscriber: drop rather than block the reader
			}
		}
		c.mu.Unlock()
	}
}

// answerServerRequest replies to the handful of requests servers send to the
// client. A server that never gets an answer will stall, so the default is a
// benign null result rather than "method not found".
func (c *Client) answerServerRequest(msg rpcMessage) {
	var result interface{}
	switch msg.Method {
	case "workspace/configuration":
		var p struct {
			Items []json.RawMessage `json:"items"`
		}
		_ = json.Unmarshal(msg.Params, &p)
		out := make([]interface{}, len(p.Items))
		for i := range out {
			out[i] = map[string]interface{}{}
		}
		result = out
	case "workspace/workspaceFolders":
		result = nil
	case "workspace/applyEdit":
		result = map[string]interface{}{"applied": false}
	case "window/showMessageRequest":
		result = nil
	default:
		// window/workDoneProgress/create, client/registerCapability,
		// client/unregisterCapability, telemetry, anything unknown.
		result = nil
	}
	_ = c.send(rpcMessage{JSONRPC: "2.0", ID: msg.ID, Result: mustRaw(result)})
}

func (c *Client) failPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- rpcMessage{Error: &rpcError{Code: -1, Message: err.Error()}}
		delete(c.pending, id)
	}
}

func mustRaw(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

func (c *Client) send(msg rpcMessage) error {
	msg.JSONRPC = "2.0"
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.stdin.Write(body)
	return err
}

// Call issues a request and waits for the matching response.
func (c *Client) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan rpcMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	rawID := json.RawMessage(strconv.FormatInt(id, 10))
	if err := c.send(rpcMessage{ID: &rawID, Method: method, Params: mustRaw(params)}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-c.exited:
		return nil, fmt.Errorf("server exited: %v", c.exitErr)
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		if resp.Result == nil {
			return json.RawMessage("null"), nil
		}
		return resp.Result, nil
	}
}

// Notify sends a notification (no response expected).
func (c *Client) Notify(method string, params interface{}) error {
	return c.send(rpcMessage{Method: method, Params: mustRaw(params)})
}

// Notifications returns a copy of every notification received so far.
func (c *Client) Notifications() []Notification {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Notification, len(c.notes))
	copy(out, c.notes)
	return out
}

// Diagnostics returns the most recent publishDiagnostics payload for a URI and
// when it arrived.
func (c *Client) Diagnostics(uri string) (json.RawMessage, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.diags[uri]
	return e.Params, e.At, ok
}

// ClearDiagnostics forgets the stored payload for a URI so a subsequent wait
// only observes freshly published results.
func (c *Client) ClearDiagnostics(uri string) {
	c.mu.Lock()
	delete(c.diags, uri)
	c.mu.Unlock()
}

// Subscribe returns a channel of notifications plus a cancel func.
func (c *Client) Subscribe() (<-chan Notification, func()) {
	ch := make(chan Notification, 256)
	c.mu.Lock()
	c.nextSub++
	id := c.nextSub
	c.subs[id] = ch
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		delete(c.subs, id)
		c.mu.Unlock()
	}
}

// PID is the server process id (0 once it has exited).
func (c *Client) PID() int {
	if c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

// Shutdown performs the polite LSP exit sequence, then kills anything left.
func (c *Client) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, _ = c.Call(ctx, "shutdown", nil)
	cancel()
	_ = c.Notify("exit", nil)
	_ = c.stdin.Close()
	select {
	case <-c.exited:
	case <-time.After(3 * time.Second):
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		<-c.exited
	}
}
