// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
)

// The assistant surface: what an AI assistant open in somebody's editor
// connects to. See journeys/agent-access.md.
//
// Served by this binary, at an address beside the rest of the API, because a
// second process deployed next to this one cannot happen inside a customer
// estate — which is the whole reason it is here rather than in a sidecar.
//
// It speaks the Model Context Protocol over Streamable HTTP: JSON-RPC messages
// posted to one address, answered as JSON. There is no event stream, because
// nothing here pushes to an assistant unasked and a held-open connection
// through a controlled desktop buys nothing.
//
// What it exposes is CHOSEN, and short. Generating a tool per operation is the
// obvious move and the wrong one: this API answers 249 of them, an assistant
// sees that list flat, and the journey records field reports of exactly that
// failure on other tools built here — the right tool present, not found, and a
// worse one used instead. The list lives in mcp_tools.go.
//
// No tool queries anything itself. Each one names a request, and that request
// is dispatched through the same mux every other caller meets, so the access
// rules, the credential's scope and the pagination are inherited rather than
// reimplemented. A tool that answered from anywhere else would show an
// assistant a different estate from the one on the screen, and the journey's
// whole test is that what it says is checkable on screen.

const (
	// mcpPath is where an assistant connects. Also named in
	// credentialMayProceed, because every message to it is a POST — including
	// the ones that only read.
	mcpPath = "/api/v1/mcp"

	// mcpProtocolVersion is the revision of the protocol this speaks. A client
	// asking for one we know gets that one back; anything else gets this, and
	// decides for itself whether it can carry on.
	mcpProtocolVersion = "2025-06-18"

	// mcpServerName is what an assistant holding several surfaces calls this
	// one when reporting whose answer it is reading.
	mcpServerName = "chef-migration-metrics"

	// mcpMaxMessageBytes bounds what will be read off the wire. A tool call is
	// a few hundred bytes; nothing legitimate approaches this.
	mcpMaxMessageBytes = 1 << 20
)

// mcpKnownProtocolVersions are the revisions this surface will answer in. A
// client that asks for one of these gets it echoed, which is how the protocol
// says a server agrees to speak an older revision.
var mcpKnownProtocolVersions = map[string]bool{
	"2025-06-18": true,
	"2025-03-26": true,
	"2024-11-05": true,
}

// JSON-RPC failure codes, from the specification. A client reads these; the
// HTTP status tells it nothing it can act on.
const (
	jsonRPCParseError     = -32700
	jsonRPCInvalidRequest = -32600
	jsonRPCMethodNotFound = -32601
	jsonRPCInvalidParams  = -32602
	jsonRPCInternalError  = -32603
)

// jsonRPCMessage is one message off the wire.
//
// ID is kept raw and echoed untouched: the protocol allows a string or a
// number, and re-encoding one as the other loses a client its match. Absent
// means a notification, which is answered with nothing at all.
type jsonRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// jsonRPCResponse is what one message is answered with: a result or a failure,
// never both, against the id that was sent.
//
// Result is left open because its shape is the method's, not the transport's —
// what comes back from listing tools and from running one have nothing in
// common, and claiming one shape for both would describe neither.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError is a refusal a client can act on. The code is from the
// specification; the message is ours.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// handleMCP serves the assistant surface.
//
// Registered behind a session like the rest of the API: the tool list names
// every capability this deployment has, which is not something to hand out
// unauthenticated inside a customer estate.
func (r *Router) handleMCP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		// Streamable HTTP, and only that. A GET here would be the SSE stream,
		// and there is nothing to send down it.
		w.Header().Set("Allow", http.MethodPost)
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"The assistant surface takes JSON-RPC messages by POST. It holds no event "+
				"stream open, because nothing here is sent unasked.")
		return
	}

	raw, err := io.ReadAll(io.LimitReader(req.Body, mcpMaxMessageBytes))
	if err != nil {
		writeJSONRPCError(w, nil, jsonRPCParseError, "The message could not be read.")
		return
	}

	var msg jsonRPCMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		writeJSONRPCError(w, nil, jsonRPCParseError,
			"The message is not JSON this surface can read.")
		return
	}

	// A notification carries no id, so there is nothing to answer to. A client
	// that got a reply would have nothing to match it against.
	if len(msg.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	result, rpcErr := r.mcpDispatch(req, msg)
	if rpcErr != nil {
		writeJSONRPCError(w, msg.ID, rpcErr.code, rpcErr.message)
		return
	}
	WriteJSON(w, http.StatusOK, jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result:  result,
	})
}

// jsonRPCFailure is a refusal in the protocol's own terms.
type jsonRPCFailure struct {
	code    int
	message string
}

// mcpDispatch answers one message.
//
// Deliberately small: initialising, listing what there is, and running one of
// them. This surface holds no resources and no prompts, and says so by
// refusing those methods rather than answering them empty — a client told
// "empty" stops asking, and a client told "no such method" knows why.
func (r *Router) mcpDispatch(req *http.Request, msg jsonRPCMessage) (any, *jsonRPCFailure) {
	switch msg.Method {
	case "initialize":
		return r.mcpInitialize(msg.Params), nil
	case "ping":
		// The protocol's keep-alive. An empty object is the whole answer.
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpOfferedTools(req)}, nil
	case "tools/call":
		return r.mcpCallTool(req, msg.Params)
	default:
		return nil, &jsonRPCFailure{jsonRPCMethodNotFound,
			fmt.Sprintf("This surface does not serve %q. It has tools and nothing else.",
				msg.Method)}
	}
}

// mcpInitialize is the first exchange: which revision of the protocol is being
// spoken, what this surface has, and what it is called.
func (r *Router) mcpInitialize(params json.RawMessage) map[string]any {
	version := mcpProtocolVersion
	var asked struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 && json.Unmarshal(params, &asked) == nil {
		if mcpKnownProtocolVersions[asked.ProtocolVersion] {
			version = asked.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			// Tools, and only tools. Declaring a capability this does not have
			// costs a client a round trip and a wrong idea of what is here.
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    mcpServerName,
			"title":   "Chef Migration Metrics",
			"version": r.version,
		},
		"instructions": "What is stopping this estate moving to the target Chef version. " +
			"Narrow before fetching: every list here is bounded and answers a page at a " +
			"time, and asking for more returns less rather than more. Our classification " +
			"of a static finding outranks the raw tool's, and what happened on a real " +
			"machine outranks both.",
	}
}

// mcpCallTool runs one tool.
//
// A tool nobody serves is refused by name rather than answered empty. An
// assistant told "nothing found" reports a clean estate; told "no such tool" it
// tries a different one, which is the difference the journey's field reports
// turn on.
func (r *Router) mcpCallTool(req *http.Request, params json.RawMessage) (any, *jsonRPCFailure) {
	var asked struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &asked); err != nil {
		return nil, &jsonRPCFailure{jsonRPCInvalidParams,
			"A tool call needs a name and its arguments."}
	}

	tool, ok := mcpToolByName(asked.Name)
	if !ok {
		return mcpToolError(fmt.Sprintf(
			"There is no tool called %q here. Ask for the list again and pick from it.",
			asked.Name)), nil
	}
	// Not offered is not callable. The listing is a courtesy to a well-behaved
	// client; this is the part that holds. The request the tool dispatches
	// meets the credential scope rule as well, so this is the second of two.
	if tool.write && !mcpMayWrite(auth.SessionFromContext(req.Context())) {
		return mcpToolError("This credential can only read. Recording a finding needs one " +
			"its owner created with write access."), nil
	}

	method, path, body, err := tool.build(asked.Arguments)
	if err != nil {
		return mcpToolError(err.Error()), nil
	}

	status, answer := r.mcpDispatchInward(req, method, path, body)
	if status < 200 || status >= 300 {
		// Carry what the service said. A switched-off feature, a refusal and a
		// misspelt name are three different things to an assistant, and only
		// the service's own words tell them apart.
		return mcpToolError(fmt.Sprintf("%s %s was refused (HTTP %d): %s",
			method, path, status, string(answer))), nil
	}

	if shaped, err := tool.shapeAnswer(asked.Arguments, answer); err != nil {
		return mcpToolError(err.Error()), nil
	} else if len(shaped) > mcpMaxAnswerBytes {
		return mcpToolError(fmt.Sprintf(
			"That answer is %d bytes, past the %d this surface will hand over in one piece. "+
				"Narrow it — %s", len(shaped), mcpMaxAnswerBytes, tool.narrowing)), nil
	} else {
		return map[string]any{
			"content": []any{map[string]any{"type": "text", "text": string(shaped)}},
			"isError": false,
		}, nil
	}
}

// mcpToolError is a tool saying the call went wrong, in the protocol's own way:
// a result, not a JSON-RPC failure, so the assistant reads it as something it
// did rather than something broken and can act on it.
func mcpToolError(message string) map[string]any {
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": message}},
		"isError": true,
	}
}

// mcpMayWrite reports whether this caller may be offered the one write there
// is.
//
// A credential says so itself, because its owner chose when they made it. A
// person at a screen has made no such choice, so their role decides — and it
// still decides underneath a credential too, in the handler.
func mcpMayWrite(info *auth.SessionInfo) bool {
	if info == nil {
		return false
	}
	if info.IsCredential() {
		return info.CredentialCanWrite
	}
	return true
}

// mcpDispatchInward runs the request a tool named, through the same mux
// everything else goes through.
//
// The headers are carried over so the request is authenticated the way the
// outer one was, and so the credential scope rule sees it: what a tool is
// really doing is settled here, on the request that acts, not on the envelope
// that carried it.
func (r *Router) mcpDispatchInward(outer *http.Request, method, path string,
	body []byte) (int, []byte) {
	inner, err := http.NewRequestWithContext(outer.Context(), method, path, bytes.NewReader(body))
	if err != nil {
		return http.StatusInternalServerError,
			[]byte(fmt.Sprintf("could not build the request for %s %s", method, path))
	}
	inner.Header = outer.Header.Clone()
	inner.Header.Set("Content-Type", "application/json")
	inner.Header.Del("Content-Length")
	inner.RemoteAddr = outer.RemoteAddr

	capture := &mcpCapture{headers: make(http.Header)}
	r.mux.ServeHTTP(capture, inner)
	return capture.statusOr200(), capture.body.Bytes()
}

// mcpCapture collects what a handler wrote, so it can be handed to an assistant
// rather than to a socket.
//
// The buffer is bounded well above the ceiling an answer is allowed to reach:
// past that the answer is refused anyway, and reading further only to throw it
// away would let one call hold a lot of memory.
type mcpCapture struct {
	headers  http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (c *mcpCapture) Header() http.Header { return c.headers }

func (c *mcpCapture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *mcpCapture) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	if room := mcpCaptureCeiling - c.body.Len(); room < len(p) {
		c.overflow = true
		if room <= 0 {
			return len(p), nil
		}
		c.body.Write(p[:room])
		return len(p), nil
	}
	return c.body.Write(p)
}

func (c *mcpCapture) statusOr200() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}

// mcpCaptureCeiling is how much of a handler's answer is held before the rest
// is dropped. Comfortably past mcpMaxAnswerBytes, so anything that reaches it
// was going to be refused for being too big regardless.
const mcpCaptureCeiling = 4 * mcpMaxAnswerBytes

// writeJSONRPCError answers in the protocol's terms.
//
// The HTTP status stays 200: a client reading JSON-RPC looks at the error
// object, and an HTTP error body it cannot parse tells it only that something
// went wrong somewhere.
func writeJSONRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	WriteJSON(w, http.StatusOK, jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonRPCError{Code: code, Message: message},
	})
}
