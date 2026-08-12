// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The assistant surface, served by this binary at /api/v1/mcp. See
// journeys/agent-access.md.
//
// These run through the real authentication middleware with a real credential
// in the header, because most of what is under test here is about what a
// particular credential is offered and allowed — which is settled between the
// header and the handler, and skipped entirely by a session pushed straight
// into the context.

// mcpRequest sends one JSON-RPC message on a credential and returns the raw
// recorder, so a test can look at the HTTP answer as well as the RPC one.
func mcpRequest(t *testing.T, router *Router, secret, method string, params any) *httptest.ResponseRecorder {
	t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		msg["params"] = params
	}
	encoded, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("encoding a %s message: %v", method, err)
	}
	return credentialRequest(t, router, secret, http.MethodPost, mcpPath, string(encoded))
}

// mcpResult sends one JSON-RPC message and returns the result object, failing
// the test if the call did not succeed at either level.
func mcpResult(t *testing.T, router *Router, secret, method string, params any) map[string]any {
	t.Helper()
	w := mcpRequest(t, router, secret, method, params)
	if w.Code != http.StatusOK {
		t.Fatalf("%s answered HTTP %d: %s", method, w.Code, w.Body.String())
	}
	var envelope struct {
		Result map[string]any  `json:"result"`
		Error  *map[string]any `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("reading what %s answered: %v (%s)", method, err, w.Body.String())
	}
	if envelope.Error != nil {
		t.Fatalf("%s failed: %v", method, *envelope.Error)
	}
	return envelope.Result
}

// mcpToolNames lists the tools this credential is offered.
func mcpToolNames(t *testing.T, router *Router, secret string) []string {
	t.Helper()
	result := mcpResult(t, router, secret, "tools/list", nil)
	listed, _ := result["tools"].([]any)
	names := make([]string, 0, len(listed))
	for _, entry := range listed {
		tool, _ := entry.(map[string]any)
		name, _ := tool["name"].(string)
		names = append(names, name)
	}
	return names
}

// mcpCallTool invokes one tool and returns the tool result object.
func mcpCallTool(t *testing.T, router *Router, secret, name string, args map[string]any) map[string]any {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	return mcpResult(t, router, secret, "tools/call",
		map[string]any{"name": name, "arguments": args})
}

// mcpToolText is the text a tool answered with, joined.
func mcpToolText(t *testing.T, result map[string]any) string {
	t.Helper()
	blocks, _ := result["content"].([]any)
	var b strings.Builder
	for _, entry := range blocks {
		block, _ := entry.(map[string]any)
		if text, ok := block["text"].(string); ok {
			b.WriteString(text)
		}
	}
	return b.String()
}

// mcpIsError reports whether the tool said the call went wrong. This is the
// only way an assistant can tell it used a tool wrongly from it having found
// nothing, which the journey names as a failure seen on other tools built here.
func mcpIsError(result map[string]any) bool {
	failed, _ := result["isError"].(bool)
	return failed
}

// "The assistant-facing surface is part of the service. Not a sidecar."
//
// A client's first message. Nothing else can happen until this is answered, so
// a wrong answer here is the whole surface being unreachable.
func TestMCP_InitializeSaysWhatItIsAndThatItHasTools(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	result := mcpResult(t, router, secret, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	})

	if version, _ := result["protocolVersion"].(string); version == "" {
		t.Error("the surface does not say which version of the protocol it speaks, so a " +
			"client cannot tell whether it can talk to it")
	}
	capabilities, _ := result["capabilities"].(map[string]any)
	if _, ok := capabilities["tools"]; !ok {
		t.Error("the surface does not declare that it has tools, so a well-behaved client " +
			"never asks for the list and the whole thing is invisible")
	}
	info, _ := result["serverInfo"].(map[string]any)
	if name, _ := info["name"].(string); name == "" {
		t.Error("the surface does not name itself, so an assistant holding several cannot " +
			"tell whose answers these are")
	}
}

// "It should be able to tell what it can ask for without being told."
//
// An assistant sees the names and the descriptions and nothing else. A tool
// that does not describe itself is one it will skip or misuse.
func TestMCP_EveryToolSaysWhatItIsForAndWhatItTakes(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", true)

	result := mcpResult(t, router, secret, "tools/list", nil)
	listed, _ := result["tools"].([]any)
	if len(listed) == 0 {
		t.Fatal("the surface offers no tools at all, so an assistant that connects to it " +
			"can do nothing with it")
	}

	for _, entry := range listed {
		tool, _ := entry.(map[string]any)
		name, _ := tool["name"].(string)
		if strings.TrimSpace(name) == "" {
			t.Errorf("a tool has no name: %v", tool)
			continue
		}
		if description, _ := tool["description"].(string); strings.TrimSpace(description) == "" {
			t.Errorf("%s says nothing about what it is for, so an assistant scanning the "+
				"list cannot tell whether it is the right one", name)
		}
		schema, _ := tool["inputSchema"].(map[string]any)
		if kind, _ := schema["type"].(string); kind != "object" {
			t.Errorf("%s does not describe its arguments as an object, which is the one "+
				"shape the protocol allows, so a client cannot build a call to it", name)
		}
	}
}

// "The list is short on purpose." An assistant picking from hundreds picks
// wrong — the journey says so from field reports of that exact failure.
//
// The number is not the point; that it stays small enough to read is. This
// fails when somebody starts generating the list from the description.
func TestMCP_TheListIsShortEnoughToRead(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", true)

	if names := mcpToolNames(t, router, secret); len(names) > 12 {
		t.Errorf("the surface offers %d tools (%v). The API has 249 operations and exposing "+
			"them is the obvious move and the wrong one: an assistant reads this list flat "+
			"and picks from it", len(names), names)
	}
}

// "I choose, when I make a credential, whether it can also write. Most will be
// read-only."
//
// The choice has to be visible in what the assistant is offered, not only in
// what it is refused. A tool it can see and cannot use is one it will keep
// trying.
func TestMCP_AReadOnlyCredentialIsNotOfferedAWriteTool(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	names := mcpToolNames(t, router, secret)
	if len(names) == 0 {
		t.Fatal("a read-only credential is offered nothing at all, so it cannot even read")
	}
	for _, name := range names {
		if name == mcpRecordFindingTool {
			t.Errorf("a read-only credential is offered %s, so an assistant will try to "+
				"record a finding and be refused, having already told its user it would",
				mcpRecordFindingTool)
		}
	}
}

// The other half: a credential its owner deliberately made write-capable is
// offered the one write there is.
func TestMCP_AWritingCredentialIsOfferedTheWriteTool(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", true)

	// The baseline, or this proves nothing: the read-only listing above and
	// this one must differ by exactly the write tool. A test that only looked
	// for the name here would pass if every credential were offered it.
	readOnly, readOnlySecret := credentialScopeFixture(t, "admin", false)
	before := len(mcpToolNames(t, readOnly, readOnlySecret))

	names := mcpToolNames(t, router, secret)
	found := false
	for _, name := range names {
		if name == mcpRecordFindingTool {
			found = true
		}
	}
	if !found {
		t.Errorf("a credential its owner made write-capable is not offered %s, so the "+
			"choice they made when creating it does nothing and the diagnosis still gets "+
			"retyped by hand", mcpRecordFindingTool)
	}
	if len(names) != before+1 {
		t.Errorf("write-capable is offered %d tools and read-only %d; the difference has to "+
			"be the one write and nothing else", len(names), before)
	}
}

// Naming the tool directly is not a way round not being offered it.
//
// The listing is a courtesy to a well-behaved client. What holds is the
// refusal, and it holds because the tool dispatches a real request that goes
// back through the same scope rule every other caller meets.
func TestMCP_AReadOnlyCredentialCannotCallTheWriteTool(t *testing.T) {
	// The baseline first: a credential that may write can record a finding
	// through this tool. Without it a broken tool would read as a refusal.
	writer, writerSecret := credentialScopeFixture(t, "admin", true)
	if result := mcpCallTool(t, writer, writerSecret, mcpRecordFindingTool, map[string]any{
		"subject_name":  "example-cookbook",
		"subject_type":  "git_repo",
		"cookbook_name": "example-cookbook",
		"verdict":       "broken",
		"reason":        "it fails to converge",
	}); mcpIsError(result) {
		t.Fatalf("a credential made to record findings cannot record one, so this test "+
			"cannot tell a refusal from a broken tool: %s", mcpToolText(t, result))
	}

	router, secret := credentialScopeFixture(t, "admin", false)
	result := mcpCallTool(t, router, secret, mcpRecordFindingTool, map[string]any{
		"subject_name":  "example-cookbook",
		"subject_type":  "git_repo",
		"cookbook_name": "example-cookbook",
		"verdict":       "broken",
		"reason":        "it fails to converge",
	})
	if !mcpIsError(result) {
		t.Errorf("a read-only credential recorded a finding by naming the tool it was not "+
			"offered, so the listing is the only thing standing between a tool and the "+
			"register: %s", mcpToolText(t, result))
	}
}

// "Every call to it is a POST, including the ones that only read."
//
// The transport is not the act. A read-only credential has to get past the
// scope rule on the envelope, or it cannot read anything at all — while the
// request the envelope carries still meets that rule, which is what the test
// above proves.
func TestMCP_TheTransportPostIsNotItselfAWrite(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	if w := mcpRequest(t, router, secret, "tools/list", nil); w.Code == http.StatusForbidden {
		t.Fatalf("a read-only credential cannot post to the assistant surface at all, so it "+
			"cannot list tools, cannot read anything, and the read-only case — which the "+
			"journey says most will be — is the one that does not work: %s", w.Body.String())
	}
}

// "It acts as me, at my level of access, and can see exactly what I can see on
// the screen and nothing else."
//
// The account's role still decides, underneath the credential's write choice.
// A credential that may write, on an account that may not, records nothing.
func TestMCP_TheAccountsLevelStillAppliesUnderneathTheTool(t *testing.T) {
	router, secret := credentialScopeFixture(t, "viewer", true)

	result := mcpCallTool(t, router, secret, mcpRecordFindingTool, map[string]any{
		"subject_name":  "example-cookbook",
		"subject_type":  "git_repo",
		"cookbook_name": "example-cookbook",
		"verdict":       "broken",
		"reason":        "it fails to converge",
	})
	if !mcpIsError(result) {
		t.Errorf("a viewer's credential recorded a finding, so handing one to a tool grants "+
			"more than its owner has on screen: %s", mcpToolText(t, result))
	}
}

// A tool call is the same request the screen makes.
//
// Nothing here reimplements a query. If a tool answered from anywhere other
// than the handler the screen uses, the assistant and the person would be
// looking at two different estates — and the journey's whole test is that what
// it says is checkable on screen.
func TestMCP_AToolAnswersWithWhatTheAddressBehindItAnswers(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	direct := credentialRequest(t, router, secret, http.MethodGet,
		"/api/v1/cookbooks?page=1&per_page="+fmt.Sprint(mcpDefaultPerPage), "")
	if direct.Code != http.StatusOK {
		t.Fatalf("the address behind the tool does not answer (%d), so this test cannot "+
			"compare anything: %s", direct.Code, direct.Body.String())
	}

	result := mcpCallTool(t, router, secret, mcpFindCookbooksTool, nil)
	if mcpIsError(result) {
		t.Fatalf("the tool failed: %s", mcpToolText(t, result))
	}
	if got := strings.TrimSpace(mcpToolText(t, result)); got != strings.TrimSpace(direct.Body.String()) {
		t.Errorf("the tool answered something other than the address behind it.\n tool: %s\ndirect: %s",
			got, direct.Body.String())
	}
}

// "The same narrowing I do on the screen has to be available to it."
func TestMCP_AToolPassesTheNarrowingThrough(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	narrowed := credentialRequest(t, router, secret, http.MethodGet,
		"/api/v1/cookbooks?page=1&per_page="+fmt.Sprint(mcpDefaultPerPage)+
			"&compatibility=incompatible&name=apache", "")
	if narrowed.Code != http.StatusOK {
		t.Fatalf("the narrowed address does not answer (%d): %s", narrowed.Code, narrowed.Body.String())
	}

	result := mcpCallTool(t, router, secret, mcpFindCookbooksTool, map[string]any{
		"compatibility": "incompatible",
		"name":          "apache",
	})
	if got := strings.TrimSpace(mcpToolText(t, result)); got != strings.TrimSpace(narrowed.Body.String()) {
		t.Errorf("narrowing through the tool does not reach the address behind it, so an "+
			"assistant has to pull everything and filter it in its own head.\n tool: %s\ndirect: %s",
			got, narrowed.Body.String())
	}
}

// "No answer is unbounded, and the caller does not have to ask for the bound."
//
// The API's own ceiling is 500 rows and a megabyte of file, which is bounded
// and still far more than an assistant can hold. So the surface has its own,
// smaller, and applies it whether or not the caller asked.
func TestMCP_AskingForMoreGetsLessNotMore(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	greedy := credentialRequest(t, router, secret, http.MethodGet,
		"/api/v1/cookbooks?page=1&per_page="+fmt.Sprint(mcpMaxPerPage), "")
	if greedy.Code != http.StatusOK {
		t.Fatalf("the address behind the tool does not answer (%d): %s", greedy.Code, greedy.Body.String())
	}

	result := mcpCallTool(t, router, secret, mcpFindCookbooksTool, map[string]any{
		"per_page": 1000000,
	})
	if got := strings.TrimSpace(mcpToolText(t, result)); got != strings.TrimSpace(greedy.Body.String()) {
		t.Errorf("a caller asked for a million and did not get the surface's ceiling, so the "+
			"bound is the caller's rather than ours.\n tool: %s\ndirect: %s",
			got, greedy.Body.String())
	}
	if mcpMaxPerPage >= maxPerPage {
		t.Errorf("the surface's ceiling (%d) is no smaller than the API's (%d), so it is not "+
			"a bound on what an assistant can be handed at all", mcpMaxPerPage, maxPerPage)
	}
}

// "It can tell when it has used one wrongly rather than reporting the empty
// answer it got."
//
// A tool nobody serves is refused by name. The alternative — an empty result —
// is the failure the journey's field reports describe.
func TestMCP_AToolNobodyServesIsRefusedByName(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	result := mcpCallTool(t, router, secret, "cmm_no_such_tool", nil)
	if !mcpIsError(result) {
		t.Fatalf("a tool that does not exist answered as though it had run: %s",
			mcpToolText(t, result))
	}
	if !strings.Contains(mcpToolText(t, result), "cmm_no_such_tool") {
		t.Errorf("the refusal does not name what was asked for, so an assistant cannot tell "+
			"which of its guesses was wrong: %s", mcpToolText(t, result))
	}
}

// A refusal from the address behind a tool reaches the assistant as a refusal,
// carrying what the service said.
//
// Run events are gated off by default and the gate answers as though the
// address does not exist. An assistant told "nothing found" would report the
// estate as clean; told "run events are not enabled" it asks a person.
func TestMCP_ARefusalBehindAToolReachesTheAssistant(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	result := mcpCallTool(t, router, secret, mcpRunFailuresTool, nil)
	if !mcpIsError(result) {
		t.Fatalf("a switched-off feature answered as though it had run, so an assistant "+
			"reads an empty estate: %s", mcpToolText(t, result))
	}
	if !strings.Contains(strings.ToLower(mcpToolText(t, result)), "not enabled") {
		t.Errorf("the refusal does not carry what the service said, so an assistant cannot "+
			"tell a switched-off feature from an empty answer: %s", mcpToolText(t, result))
	}
}

// Streamable HTTP, and only that. Nothing here pushes to an assistant unasked,
// so there is nothing to hold a connection open for.
func TestMCP_ThereIsNoStreamToHoldOpen(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	w := credentialRequest(t, router, secret, http.MethodGet, mcpPath, "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("a GET on the assistant surface answered %d rather than refusing the "+
			"method; this surface speaks Streamable HTTP and holds nothing open", w.Code)
	}
}

// The surface is behind a session like everything else. Without one it names
// every capability this deployment has, which is not something to hand out
// unauthenticated inside a customer estate.
func TestMCP_NothingIsOfferedWithoutACredential(t *testing.T) {
	router, _ := credentialScopeFixture(t, "admin", false)

	req := httptest.NewRequest(http.MethodPost, mcpPath,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("the assistant surface answered %d to a caller with no credential, so it "+
			"lists what this deployment can do to anybody who asks", w.Code)
	}
}

// A notification carries no id and gets no answer — a client that waited for
// one would hang.
func TestMCP_ANotificationIsNotAnswered(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	w := credentialRequest(t, router, secret, http.MethodPost, mcpPath,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if w.Code != http.StatusAccepted {
		t.Errorf("a notification was answered %d rather than accepted silently", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) != "" {
		t.Errorf("a notification got a reply body (%s), which a client has no id to match "+
			"against", w.Body.String())
	}
}

// Malformed input is refused in the protocol's own terms, not the HTTP layer's:
// a client reading JSON-RPC cannot see an HTTP error body.
func TestMCP_SomethingUnreadableIsRefusedInTheProtocolsOwnTerms(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	w := credentialRequest(t, router, secret, http.MethodPost, mcpPath, `{"not json`)
	var envelope struct {
		JSONRPC string `json:"jsonrpc"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unreadable input was not answered with a readable message: %v (%s)",
			err, w.Body.String())
	}
	if envelope.Error == nil || envelope.Error.Code != jsonRPCParseError {
		t.Errorf("unreadable input was not refused as a parse error, so a client cannot tell "+
			"it sent nonsense from the service being broken: %s", w.Body.String())
	}
}

// A method the surface does not implement is refused as such rather than
// ignored.
func TestMCP_AMethodItDoesNotHaveIsRefused(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	w := mcpRequest(t, router, secret, "resources/list", nil)
	var envelope struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("reading the answer: %v (%s)", err, w.Body.String())
	}
	if envelope.Error == nil || envelope.Error.Code != jsonRPCMethodNotFound {
		t.Errorf("a method this surface does not have was not refused as missing, so a "+
			"client keeps trying it: %s", w.Body.String())
	}
}

// Every tool this surface offers must reach an address this service really
// serves. A tool naming a path nothing routes would answer "not found" to an
// assistant that had done nothing wrong — the same false lead the description's
// own drift tests exist to stop, arriving by a different route.
func TestMCP_EveryToolReachesAnAddressThatIsReallyServed(t *testing.T) {
	router := newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0"))

	for _, tool := range mcpTools {
		probe := httptest.NewRequest(http.MethodGet, tool.probePath, nil)
		if _, matched := router.mux.Handler(probe); matched == "/" {
			t.Errorf("%s reaches %q, which nothing serves — it falls through to the "+
				"frontend, so an assistant gets a web page where it expected an answer",
				tool.name, tool.probePath)
		}
	}
}
