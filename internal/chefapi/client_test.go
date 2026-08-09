package chefapi

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// generateTestKey creates a PEM-encoded RSA private key for testing.
func generateTestKey(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test RSA key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}
	return pem.EncodeToMemory(block)
}

// generateTestKeyPKCS8 creates a PEM-encoded PKCS#8 RSA private key for testing.
func generateTestKeyPKCS8(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test RSA key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal PKCS#8 key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	return pem.EncodeToMemory(block)
}

// newTestClient creates a Client backed by a test HTTP server.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{
		ServerURL:     server.URL + "/organizations/testorg",
		ClientName:    "test-client",
		PrivateKeyPEM: generateTestKey(t),
		OrgName:       "testorg",
		AppVersion:    "1.0.0-test",
		HTTPClient:    server.Client(),
	})
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return client, server
}

// ---------------------------------------------------------------------------
// NewClient tests
// ---------------------------------------------------------------------------

func TestNewClient_Valid(t *testing.T) {
	c, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "metrics",
		PrivateKeyPEM: generateTestKey(t),
		OrgName:       "myorg",
		AppVersion:    "2.0.0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.clientName != "metrics" {
		t.Errorf("expected clientName 'metrics', got %q", c.clientName)
	}
	if c.userAgent != "chef-migration-metrics/2.0.0 (org:myorg)" {
		t.Errorf("unexpected user agent: %q", c.userAgent)
	}
}

func TestNewClient_PKCS8Key(t *testing.T) {
	_, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "metrics",
		PrivateKeyPEM: generateTestKeyPKCS8(t),
	})
	if err != nil {
		t.Fatalf("unexpected error with PKCS#8 key: %v", err)
	}
}

func TestNewClient_MissingServerURL(t *testing.T) {
	_, err := NewClient(ClientConfig{
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
	})
	if err == nil {
		t.Fatal("expected error for missing ServerURL")
	}
	if !strings.Contains(err.Error(), "ServerURL is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewClient_MissingClientName(t *testing.T) {
	_, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		PrivateKeyPEM: generateTestKey(t),
	})
	if err == nil {
		t.Fatal("expected error for missing ClientName")
	}
	if !strings.Contains(err.Error(), "ClientName is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewClient_MissingKey(t *testing.T) {
	_, err := NewClient(ClientConfig{
		ServerURL:  "https://chef.example.com/organizations/myorg",
		ClientName: "test",
	})
	if err == nil {
		t.Fatal("expected error for missing PrivateKeyPEM")
	}
	if !strings.Contains(err.Error(), "PrivateKeyPEM is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewClient_InvalidKey(t *testing.T) {
	_, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: []byte("not a pem key"),
	})
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "failed to decode PEM") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewClient_UnsupportedPEMType(t *testing.T) {
	block := &pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("fake")}
	pemData := pem.EncodeToMemory(block)

	_, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: pemData,
	})
	if err == nil {
		t.Fatal("expected error for unsupported PEM type")
	}
	if !strings.Contains(err.Error(), "unsupported PEM block type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewClient_DefaultsApplied(t *testing.T) {
	c, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.userAgent, "dev") {
		t.Errorf("expected 'dev' in user agent, got %q", c.userAgent)
	}
	if !strings.Contains(c.userAgent, "unknown") {
		t.Errorf("expected 'unknown' org in user agent, got %q", c.userAgent)
	}
	if c.httpClient == http.DefaultClient {
		t.Error("must not use http.DefaultClient — it has no timeout")
	}
	if c.httpClient.Timeout <= 0 {
		t.Error("expected a bounded default HTTP client")
	}
}

func TestNewClient_SSLVerify_DefaultUsesVerifyingClient(t *testing.T) {
	// When SSLVerify is nil (not set), TLS verification stays on: the
	// transport must carry no InsecureSkipVerify override.
	c, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.httpClient.Transport)
	}
	if tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected TLS verification to remain enabled when SSLVerify is nil")
	}
}

func TestNewClient_SSLVerify_ExplicitTrueUsesVerifyingClient(t *testing.T) {
	sslVerify := true
	c, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		SSLVerify:     &sslVerify,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.httpClient.Transport)
	}
	if tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected TLS verification to remain enabled when SSLVerify is true")
	}
}

func TestNewClient_SSLVerify_FalseSkipsVerification(t *testing.T) {
	sslVerify := false
	c, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		SSLVerify:     &sslVerify,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.httpClient == http.DefaultClient {
		t.Fatal("expected a custom HTTP client when SSLVerify is false, got http.DefaultClient")
	}
	transport, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.httpClient.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig to be set")
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true when SSLVerify is false")
	}
}

func TestNewClient_SSLVerify_FalseWithCustomHTTPClient(t *testing.T) {
	// When the caller provides their own HTTPClient, it should be used
	// even when SSLVerify is false (the caller is responsible for their
	// own transport configuration).
	sslVerify := false
	custom := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		},
	}
	c, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		SSLVerify:     &sslVerify,
		HTTPClient:    custom,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.httpClient != custom {
		t.Error("expected the custom HTTPClient to be preserved when explicitly provided")
	}
}

func TestNewClient_SSLVerify_FalseEmitsWarning(t *testing.T) {
	sslVerify := false
	var warnings []string
	_, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		SSLVerify:     &sslVerify,
		WarnFunc:      func(msg string) { warnings = append(warnings, msg) },
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning when SSLVerify is false, got none")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "ssl") || strings.Contains(strings.ToLower(w), "insecure") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warning did not mention ssl/insecure: %v", warnings)
	}
}

func TestNewClient_SSLVerify_TrueNoWarning(t *testing.T) {
	sslVerify := true
	var warnings []string
	_, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		SSLVerify:     &sslVerify,
		WarnFunc:      func(msg string) { warnings = append(warnings, msg) },
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings when SSLVerify is true, got: %v", warnings)
	}
}

func TestNewClient_TrailingSlashStripped(t *testing.T) {
	c, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg/",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.HasSuffix(c.serverURL.Path, "/") {
		t.Errorf("trailing slash not stripped from path: %q", c.serverURL.Path)
	}
}

// ---------------------------------------------------------------------------
// Request signing tests
// ---------------------------------------------------------------------------

func TestSignRequest_HeadersPresent(t *testing.T) {
	client, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Check all required headers are present.
		requiredHeaders := []string{
			"Accept",
			"User-Agent",
			"X-Chef-Version",
			"X-Ops-Sign",
			"X-Ops-Timestamp",
			"X-Ops-Userid",
			"X-Ops-Content-Hash",
			"X-Ops-Server-Api-Version",
			"X-Ops-Authorization-1",
		}
		for _, h := range requiredHeaders {
			if r.Header.Get(h) == "" {
				t.Errorf("missing required header: %s", h)
			}
		}

		// Check specific header values.
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept: expected application/json, got %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("X-Ops-Sign") != "version=1.3" {
			t.Errorf("X-Ops-Sign: expected version=1.3, got %q", r.Header.Get("X-Ops-Sign"))
		}
		if r.Header.Get("X-Ops-Userid") != "test-client" {
			t.Errorf("X-Ops-Userid: expected test-client, got %q", r.Header.Get("X-Ops-Userid"))
		}
		if r.Header.Get("X-Ops-Server-Api-Version") != "1" {
			t.Errorf("X-Ops-Server-Api-Version: expected 1, got %q", r.Header.Get("X-Ops-Server-Api-Version"))
		}
		if r.Header.Get("X-Chef-Version") != "17.0.0" {
			t.Errorf("X-Chef-Version: expected 17.0.0, got %q", r.Header.Get("X-Chef-Version"))
		}

		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})
	_ = server

	_, err := client.doRequest(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSignRequest_UserAgent(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != "chef-migration-metrics/1.0.0-test (org:testorg)" {
			t.Errorf("unexpected User-Agent: %q", ua)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	_, err := client.doRequest(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSignRequest_ContentTypeOnPOST(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json on POST, got %q", ct)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	_, err := client.doRequest(context.Background(), "POST", "/test", []byte(`{"key":"value"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSignRequest_TimestampFormat(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ts := r.Header.Get("X-Ops-Timestamp")
		_, err := time.Parse("2006-01-02T15:04:05Z", ts)
		if err != nil {
			t.Errorf("X-Ops-Timestamp not in ISO-8601 UTC format: %q (%v)", ts, err)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	_, err := client.doRequest(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSignRequest_AuthHeadersSplit(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// 2048-bit RSA signature → 256 bytes → ~344 base64 chars → 6 segments of 60
		for i := 1; i <= 6; i++ {
			h := fmt.Sprintf("X-Ops-Authorization-%d", i)
			v := r.Header.Get(h)
			if v == "" {
				t.Errorf("missing %s", h)
				continue
			}
			if i < 6 && len(v) != 60 {
				t.Errorf("%s: expected 60 chars, got %d", h, len(v))
			}
			if i == 6 && len(v) > 60 {
				t.Errorf("%s: expected <= 60 chars, got %d", h, len(v))
			}
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	_, err := client.doRequest(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// splitString tests
// ---------------------------------------------------------------------------

func TestSplitString(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  int // number of chunks
	}{
		{"", 10, 0},
		{"hello", 10, 1},
		{"hello", 2, 3},
		{"abcdef", 3, 2},
		{"abcdef", 6, 1},
		{"abcdef", 1, 6},
	}

	for _, tt := range tests {
		chunks := splitString(tt.input, tt.n)
		if len(chunks) != tt.want {
			t.Errorf("splitString(%q, %d): expected %d chunks, got %d", tt.input, tt.n, tt.want, len(chunks))
		}
		// Verify round-trip.
		if joined := strings.Join(chunks, ""); joined != tt.input {
			t.Errorf("splitString(%q, %d): round-trip failed, got %q", tt.input, tt.n, joined)
		}
		// Verify no chunk exceeds n.
		for i, c := range chunks {
			if len(c) > tt.n {
				t.Errorf("splitString(%q, %d): chunk[%d] has length %d > %d", tt.input, tt.n, i, len(c), tt.n)
			}
		}
	}
}

func TestSplitString_ZeroN(t *testing.T) {
	chunks := splitString("abc", 0)
	if len(chunks) != 1 || chunks[0] != "abc" {
		t.Errorf("expected single chunk with full string, got %v", chunks)
	}
}

// ---------------------------------------------------------------------------
// API error handling tests
// ---------------------------------------------------------------------------

func TestDoRequest_Non2xxReturnsAPIError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"unauthorized"}`))
	})

	_, err := client.doRequest(context.Background(), "GET", "/test", nil)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", apiErr.StatusCode)
	}
	if apiErr.Method != "GET" {
		t.Errorf("expected method GET, got %q", apiErr.Method)
	}
	if !strings.Contains(apiErr.Body, "unauthorized") {
		t.Errorf("expected body containing 'unauthorized', got %q", apiErr.Body)
	}
}

func TestAPIError_Error(t *testing.T) {
	e := &APIError{StatusCode: 404, Method: "GET", Path: "/nodes/missing", Body: "not found"}
	msg := e.Error()
	if !strings.Contains(msg, "GET") || !strings.Contains(msg, "404") || !strings.Contains(msg, "not found") {
		t.Errorf("unexpected error string: %q", msg)
	}
}

func TestAPIError_IsRetryable(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{200, false},
		{401, false},
		{403, false},
		{404, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
	}
	for _, tt := range tests {
		e := &APIError{StatusCode: tt.status}
		if got := e.IsRetryable(); got != tt.want {
			t.Errorf("IsRetryable(%d): expected %v, got %v", tt.status, tt.want, got)
		}
	}
}

func TestAPIError_IsNotFound(t *testing.T) {
	if (&APIError{StatusCode: 404}).IsNotFound() != true {
		t.Error("404 should be not found")
	}
	if (&APIError{StatusCode: 200}).IsNotFound() != false {
		t.Error("200 should not be not found")
	}
}

// ---------------------------------------------------------------------------
// Partial search tests
// ---------------------------------------------------------------------------

func TestPartialSearch(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/search/node") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		q := r.URL.Query()
		if q.Get("q") != "*:*" {
			t.Errorf("unexpected query: %q", q.Get("q"))
		}
		if q.Get("rows") != "100" {
			t.Errorf("unexpected rows: %q", q.Get("rows"))
		}
		if q.Get("start") != "0" {
			t.Errorf("unexpected start: %q", q.Get("start"))
		}

		// Check request body is valid JSON with expected keys.
		body, _ := io.ReadAll(r.Body)
		var attrs map[string][]string
		if err := json.Unmarshal(body, &attrs); err != nil {
			t.Errorf("invalid request body JSON: %v", err)
		}
		if _, ok := attrs["name"]; !ok {
			t.Error("expected 'name' in request body")
		}

		resp := SearchResult{
			Total: 2,
			Start: 0,
			Rows: []SearchResultRow{
				{URL: "https://chef/nodes/node1", Data: map[string]interface{}{"name": "node1"}},
				{URL: "https://chef/nodes/node2", Data: map[string]interface{}{"name": "node2"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	result, err := client.PartialSearch(ctx(), "node", "*:*", 100, 0, PartialSearchQuery{
		"name": {"name"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("expected total 2, got %d", result.Total)
	}
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}
}

func TestPartialSearch_ServerError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal"}`))
	})

	_, err := client.PartialSearch(ctx(), "node", "*:*", 100, 0, PartialSearchQuery{})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected 500, got %d", apiErr.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// NodeSearchAttributes tests
// ---------------------------------------------------------------------------

func TestNodeSearchAttributes(t *testing.T) {
	attrs := NodeSearchAttributes()
	expected := []string{
		"name", "chef_environment", "chef_version", "platform",
		"platform_version", "platform_family", "kernel_os_info_caption",
		"lsb_description", "filesystem", "cookbooks",
		"run_list", "roles", "tags", "policy_name", "policy_group", "ohai_time",
		"chef_migration",
	}
	for _, key := range expected {
		if _, ok := attrs[key]; !ok {
			t.Errorf("missing expected attribute key: %q", key)
		}
	}
	if len(attrs) != len(expected) {
		t.Errorf("expected %d attributes, got %d", len(expected), len(attrs))
	}
}

// TestNodeSearchAttributes_NoAutomaticPrefix verifies that partial search
// attribute paths do NOT include "automatic" as the first path element.
//
// Chef search (both full and partial) returns a flattened structure where
// automatic attributes are hoisted to the root of each node's data object.
// For example, the node attribute stored at automatic.platform is accessed
// as just ["platform"] in partial search, not ["automatic", "platform"].
//
// Fetching a node directly (GET /nodes/:name) does NOT hoist — attributes
// remain nested under "automatic", "default", "normal", "override". But
// partial search always works against the hoisted/merged view.
//
// If any path starts with "automatic", the Chef server will look for a
// literal top-level key called "automatic" in the hoisted structure, which
// does not exist, and return null for that attribute.
func TestNodeSearchAttributes_NoAutomaticPrefix(t *testing.T) {
	attrs := NodeSearchAttributes()
	for key, path := range attrs {
		if len(path) > 0 && path[0] == "automatic" {
			t.Errorf("attribute %q has path %v starting with \"automatic\" — "+
				"partial search uses the hoisted structure where automatic "+
				"attributes are at the root; remove the \"automatic\" prefix",
				key, path)
		}
	}
}

// ---------------------------------------------------------------------------
// NodeSearchAttributesWithExtra tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// NodeData chef_migration accessor tests
// ---------------------------------------------------------------------------

func TestNodeData_MigrationFields_Nil(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"name": "node1",
	})
	if nd.HasMigrationData() {
		t.Error("expected HasMigrationData() false when chef_migration absent")
	}
	if nd.MigrationState() != "" {
		t.Errorf("expected empty MigrationState, got %q", nd.MigrationState())
	}
	if nd.ActiveChefVersion() != "" {
		t.Errorf("expected empty ActiveChefVersion, got %q", nd.ActiveChefVersion())
	}
	if nd.DormantInstalled() {
		t.Error("expected DormantInstalled false when chef_migration absent")
	}
	if nd.DormantChefVersion() != "" {
		t.Errorf("expected empty DormantChefVersion, got %q", nd.DormantChefVersion())
	}
	if nd.TargetVersion() != "" {
		t.Errorf("expected empty TargetVersion, got %q", nd.TargetVersion())
	}
	if nd.TargetExecutionTime() != "" {
		t.Errorf("expected empty TargetExecutionTime, got %q", nd.TargetExecutionTime())
	}
	if nd.TargetConvergeStatus() != "" {
		t.Errorf("expected empty TargetConvergeStatus, got %q", nd.TargetConvergeStatus())
	}
}

func TestNodeData_MigrationFields_Partial(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"name": "node2",
		"chef_migration": map[string]interface{}{
			"migration_state":      "hab_dormant",
			"active_chef_version":  "16.18.30",
			"dormant_installed":    true,
			"dormant_chef_version": "19.3.15",
		},
	})
	if !nd.HasMigrationData() {
		t.Error("expected HasMigrationData() true")
	}
	if nd.MigrationState() != "hab_dormant" {
		t.Errorf("expected hab_dormant, got %q", nd.MigrationState())
	}
	if nd.ActiveChefVersion() != "16.18.30" {
		t.Errorf("expected 16.18.30, got %q", nd.ActiveChefVersion())
	}
	if !nd.DormantInstalled() {
		t.Error("expected DormantInstalled true")
	}
	if nd.DormantChefVersion() != "19.3.15" {
		t.Errorf("expected 19.3.15, got %q", nd.DormantChefVersion())
	}
	// Speculative converge fields not set.
	if nd.TargetVersion() != "" {
		t.Errorf("expected empty TargetVersion, got %q", nd.TargetVersion())
	}
	if nd.TargetExecutionTime() != "" {
		t.Errorf("expected empty TargetExecutionTime, got %q", nd.TargetExecutionTime())
	}
	if nd.TargetConvergeStatus() != "" {
		t.Errorf("expected empty TargetConvergeStatus, got %q", nd.TargetConvergeStatus())
	}
}

func TestNodeData_MigrationFields_Full(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"name": "node3",
		"chef_migration": map[string]interface{}{
			"migration_state":        "hab_dormant",
			"active_chef_version":    "16.18.30",
			"dormant_installed":      true,
			"dormant_chef_version":   "19.3.15",
			"target_version":         "19.3.15",
			"target_execution_time":  "2026-06-02T22:00:00Z",
			"target_converge_status": "success",
		},
	})
	if !nd.HasMigrationData() {
		t.Error("expected HasMigrationData() true")
	}
	if nd.MigrationState() != "hab_dormant" {
		t.Errorf("expected hab_dormant, got %q", nd.MigrationState())
	}
	if nd.ActiveChefVersion() != "16.18.30" {
		t.Errorf("expected 16.18.30, got %q", nd.ActiveChefVersion())
	}
	if !nd.DormantInstalled() {
		t.Error("expected DormantInstalled true")
	}
	if nd.DormantChefVersion() != "19.3.15" {
		t.Errorf("expected 19.3.15, got %q", nd.DormantChefVersion())
	}
	if nd.TargetVersion() != "19.3.15" {
		t.Errorf("expected 19.3.15, got %q", nd.TargetVersion())
	}
	if nd.TargetExecutionTime() != "2026-06-02T22:00:00Z" {
		t.Errorf("expected 2026-06-02T22:00:00Z, got %q", nd.TargetExecutionTime())
	}
	if nd.TargetConvergeStatus() != "success" {
		t.Errorf("expected success, got %q", nd.TargetConvergeStatus())
	}
}

func TestNodeData_MigrationFields_WrongType(t *testing.T) {
	// chef_migration is not a map — should be handled gracefully.
	nd := NewNodeData(map[string]interface{}{
		"name":           "node4",
		"chef_migration": "not-a-map",
	})
	if nd.HasMigrationData() {
		t.Error("expected HasMigrationData() false when chef_migration is wrong type")
	}
	if nd.MigrationState() != "" {
		t.Errorf("expected empty MigrationState, got %q", nd.MigrationState())
	}
}

func TestNodeSearchAttributes_IncludesChefMigration(t *testing.T) {
	attrs := NodeSearchAttributes()
	path, ok := attrs["chef_migration"]
	if !ok {
		t.Fatal("chef_migration key missing from NodeSearchAttributes")
	}
	if len(path) != 1 || path[0] != "chef_migration" {
		t.Errorf("expected path [\"chef_migration\"], got %v", path)
	}
}

func TestNodeSearchAttributesWithExtra_Nil(t *testing.T) {
	attrs := NodeSearchAttributesWithExtra(nil)
	base := NodeSearchAttributes()
	if len(attrs) != len(base) {
		t.Errorf("expected %d attributes with nil extra, got %d", len(base), len(attrs))
	}
}

func TestNodeSearchAttributesWithExtra_CMDBKeys(t *testing.T) {
	extra := map[string][]string{
		"itil.cmdb.node":     {"itil", "cmdb", "node"},
		"itil.cmdb.cookbook": {"itil", "cmdb", "cookbook"},
		"itil.cmdb.profile":  {"itil", "cmdb", "profile"},
		"itil.cmdb.role":     {"itil", "cmdb", "role"},
	}
	attrs := NodeSearchAttributesWithExtra(extra)
	base := NodeSearchAttributes()
	if len(attrs) != len(base)+4 {
		t.Errorf("expected %d attributes, got %d", len(base)+4, len(attrs))
	}
	for key, expectedPath := range extra {
		path, ok := attrs[key]
		if !ok {
			t.Errorf("missing expected extra key %q", key)
			continue
		}
		if len(path) != len(expectedPath) {
			t.Errorf("key %q: expected path length %d, got %d", key, len(expectedPath), len(path))
			continue
		}
		for i, segment := range expectedPath {
			if path[i] != segment {
				t.Errorf("key %q: path[%d] expected %q, got %q", key, i, segment, path[i])
			}
		}
	}
	// Standard keys should still be present.
	for _, stdKey := range []string{"name", "platform", "ohai_time", "roles"} {
		if _, ok := attrs[stdKey]; !ok {
			t.Errorf("standard key %q missing after merge", stdKey)
		}
	}
}

func TestNodeSearchAttributesWithExtra_CollisionProtection(t *testing.T) {
	// Attempting to override a standard key should be silently ignored.
	extra := map[string][]string{
		"name":           {"itil", "cmdb", "evil"},  // collides with standard "name"
		"ohai_time":      {"override", "ohai_time"}, // collides with standard "ohai_time"
		"itil.cmdb.node": {"itil", "cmdb", "node"},  // new key, should be added
	}
	attrs := NodeSearchAttributesWithExtra(extra)
	base := NodeSearchAttributes()

	// Only one extra key should have been added (itil.cmdb.node).
	if len(attrs) != len(base)+1 {
		t.Errorf("expected %d attributes (base + 1 non-colliding), got %d", len(base)+1, len(attrs))
	}

	// "name" should still have its original path, not the override.
	namePath := attrs["name"]
	if len(namePath) != 1 || namePath[0] != "name" {
		t.Errorf("standard 'name' key was overwritten: got path %v", namePath)
	}

	// "ohai_time" should still have its original path.
	ohaiPath := attrs["ohai_time"]
	if len(ohaiPath) != 1 || ohaiPath[0] != "ohai_time" {
		t.Errorf("standard 'ohai_time' key was overwritten: got path %v", ohaiPath)
	}

	// The new CMDB key should be present.
	if _, ok := attrs["itil.cmdb.node"]; !ok {
		t.Error("expected 'itil.cmdb.node' key to be added")
	}
}

func TestNodeSearchAttributesWithExtra_EmptyMap(t *testing.T) {
	attrs := NodeSearchAttributesWithExtra(map[string][]string{})
	base := NodeSearchAttributes()
	if len(attrs) != len(base) {
		t.Errorf("expected %d attributes with empty extra, got %d", len(base), len(attrs))
	}
}

// ---------------------------------------------------------------------------
// CollectAllNodes tests (sequential)
// ---------------------------------------------------------------------------

func TestCollectAllNodes_SinglePage(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		resp := SearchResult{
			Total: 2,
			Start: 0,
			Rows: []SearchResultRow{
				{Data: map[string]interface{}{"name": "node1"}},
				{Data: map[string]interface{}{"name": "node2"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	rows, err := client.CollectAllNodes(ctx(), 1000, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestCollectAllNodes_MultiplePages(t *testing.T) {
	var requestCount int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		start := r.URL.Query().Get("start")

		var resp SearchResult
		switch start {
		case "0":
			resp = SearchResult{
				Total: 5,
				Start: 0,
				Rows: []SearchResultRow{
					{Data: map[string]interface{}{"name": "node1"}},
					{Data: map[string]interface{}{"name": "node2"}},
				},
			}
		case "2":
			resp = SearchResult{
				Total: 5,
				Start: 2,
				Rows: []SearchResultRow{
					{Data: map[string]interface{}{"name": "node3"}},
					{Data: map[string]interface{}{"name": "node4"}},
				},
			}
		case "4":
			resp = SearchResult{
				Total: 5,
				Start: 4,
				Rows: []SearchResultRow{
					{Data: map[string]interface{}{"name": "node5"}},
				},
			}
		default:
			t.Errorf("unexpected start value: %s", start)
			w.WriteHeader(500)
			return
		}
		json.NewEncoder(w).Encode(resp)
	})

	rows, err := client.CollectAllNodes(ctx(), 2, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 5 {
		t.Errorf("expected 5 rows, got %d", len(rows))
	}
	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}

func TestCollectAllNodes_DefaultPageSize(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		rows := r.URL.Query().Get("rows")
		if rows != "1000" {
			t.Errorf("expected default page size 1000, got %s", rows)
		}
		json.NewEncoder(w).Encode(SearchResult{Total: 0, Rows: []SearchResultRow{}})
	})

	_, err := client.CollectAllNodes(ctx(), 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CollectAllNodesConcurrent tests
// ---------------------------------------------------------------------------

func TestCollectAllNodesConcurrent_SinglePage(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		resp := SearchResult{
			Total: 1,
			Start: 0,
			Rows: []SearchResultRow{
				{Data: map[string]interface{}{"name": "node1"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	rows, err := client.CollectAllNodesConcurrent(ctx(), 1000, 5, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(rows))
	}
}

func TestCollectAllNodesConcurrent_MultiplePages(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")

		var resp SearchResult
		switch start {
		case "0":
			resp = SearchResult{Total: 6, Start: 0, Rows: []SearchResultRow{
				{Data: map[string]interface{}{"name": "n1"}},
				{Data: map[string]interface{}{"name": "n2"}},
			}}
		case "2":
			resp = SearchResult{Total: 6, Start: 2, Rows: []SearchResultRow{
				{Data: map[string]interface{}{"name": "n3"}},
				{Data: map[string]interface{}{"name": "n4"}},
			}}
		case "4":
			resp = SearchResult{Total: 6, Start: 4, Rows: []SearchResultRow{
				{Data: map[string]interface{}{"name": "n5"}},
				{Data: map[string]interface{}{"name": "n6"}},
			}}
		default:
			w.WriteHeader(500)
			return
		}
		json.NewEncoder(w).Encode(resp)
	})

	rows, err := client.CollectAllNodesConcurrent(ctx(), 2, 3, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 6 {
		t.Errorf("expected 6 rows, got %d", len(rows))
	}

	// Verify order — pages should be assembled in page order.
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = NewNodeData(r.Data).Name()
	}
	for i, expected := range []string{"n1", "n2", "n3", "n4", "n5", "n6"} {
		if names[i] != expected {
			t.Errorf("row[%d]: expected %q, got %q", i, expected, names[i])
		}
	}
}

func TestCollectAllNodesConcurrent_ErrorOnSubsequentPage(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		if start == "0" {
			json.NewEncoder(w).Encode(SearchResult{Total: 4, Start: 0, Rows: []SearchResultRow{
				{Data: map[string]interface{}{"name": "n1"}},
				{Data: map[string]interface{}{"name": "n2"}},
			}})
			return
		}
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"boom"}`))
	})

	_, err := client.CollectAllNodesConcurrent(ctx(), 2, 2, nil)
	if err == nil {
		t.Fatal("expected error when subsequent page fails")
	}
}

func TestCollectAllNodesConcurrent_ContextCancelled(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		if start == "0" {
			json.NewEncoder(w).Encode(SearchResult{Total: 100, Start: 0, Rows: []SearchResultRow{
				{Data: map[string]interface{}{"name": "n1"}},
			}})
			return
		}
		// Slow response for other pages.
		time.Sleep(5 * time.Second)
		w.WriteHeader(200)
	})

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.CollectAllNodesConcurrent(ctxTimeout, 1, 5, nil)
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// ---------------------------------------------------------------------------
// GetRoles tests
// ---------------------------------------------------------------------------

func TestGetRoles(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/roles") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"base":"https://chef/roles/base","webserver":"https://chef/roles/webserver"}`))
	})

	roles, err := client.GetRoles(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}

func TestGetRoles_Empty(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	})

	roles, err := client.GetRoles(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("expected 0 roles, got %d", len(roles))
	}
}

// ---------------------------------------------------------------------------
// GetRole tests
// ---------------------------------------------------------------------------

func TestGetRole(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/roles/base") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{
			"name": "base",
			"run_list": ["recipe[ntp::default]", "role[monitoring]"],
			"env_run_lists": {"production": ["recipe[ntp::default]"]},
			"description": "Base role"
		}`))
	})

	role, err := client.GetRole(ctx(), "base")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role.Name != "base" {
		t.Errorf("expected name 'base', got %q", role.Name)
	}
	if len(role.RunList) != 2 {
		t.Errorf("expected 2 run_list entries, got %d", len(role.RunList))
	}
	if role.Description != "Base role" {
		t.Errorf("unexpected description: %q", role.Description)
	}
	if len(role.EnvRunLists) != 1 {
		t.Errorf("expected 1 env_run_list, got %d", len(role.EnvRunLists))
	}
}

func TestGetRole_NotFound(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"not found"}`))
	})

	_, err := client.GetRole(ctx(), "missing")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !apiErr.IsNotFound() {
		t.Error("expected IsNotFound to be true")
	}
}

// ---------------------------------------------------------------------------
// GetCookbooks tests
// ---------------------------------------------------------------------------

func TestGetCookbooks(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.String(), "num_versions=all") {
			t.Error("expected num_versions=all query parameter")
		}
		w.Write([]byte(`{
			"nginx": {
				"url": "https://chef/cookbooks/nginx",
				"versions": [
					{"url": "https://chef/cookbooks/nginx/5.1.0", "version": "5.1.0"},
					{"url": "https://chef/cookbooks/nginx/4.0.0", "version": "4.0.0"}
				]
			},
			"apt": {
				"url": "https://chef/cookbooks/apt",
				"versions": [
					{"url": "https://chef/cookbooks/apt/7.4.0", "version": "7.4.0"}
				]
			}
		}`))
	})

	cbs, err := client.GetCookbooks(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cbs) != 2 {
		t.Errorf("expected 2 cookbooks, got %d", len(cbs))
	}
	nginx, ok := cbs["nginx"]
	if !ok {
		t.Fatal("missing nginx cookbook")
	}
	if len(nginx.Versions) != 2 {
		t.Errorf("expected 2 nginx versions, got %d", len(nginx.Versions))
	}
	if nginx.Versions[0].Version != "5.1.0" {
		t.Errorf("unexpected first nginx version: %q", nginx.Versions[0].Version)
	}
}

// ---------------------------------------------------------------------------
// GetCookbookVersion tests
// ---------------------------------------------------------------------------

func TestGetCookbookVersion(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/cookbooks/nginx/5.1.0") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"cookbook_name":"nginx","version":"5.1.0","metadata":{}}`))
	})

	raw, err := client.GetCookbookVersion(ctx(), "nginx", "5.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(raw), "nginx") {
		t.Error("expected response to contain 'nginx'")
	}
}

// ---------------------------------------------------------------------------
// GetCookbookVersionManifest tests
// ---------------------------------------------------------------------------

func TestGetCookbookVersionManifest(t *testing.T) {
	manifest := `{
		"cookbook_name": "nginx",
		"name": "nginx-5.1.0",
		"version": "5.1.0",
		"metadata": {},
		"recipes": [
			{"name": "default.rb", "path": "recipes/default.rb", "checksum": "abc123", "specificity": "default", "url": "https://bookshelf.example.com/files/abc123"}
		],
		"attributes": [
			{"name": "default.rb", "path": "attributes/default.rb", "checksum": "def456", "specificity": "default", "url": "https://bookshelf.example.com/files/def456"}
		],
		"definitions": [],
		"libraries": [],
		"files": [],
		"templates": [],
		"resources": [],
		"providers": [],
		"root_files": [
			{"name": "metadata.rb", "path": "metadata.rb", "checksum": "ghi789", "specificity": "default", "url": "https://bookshelf.example.com/files/ghi789"}
		]
	}`

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/cookbooks/nginx/5.1.0") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(manifest))
	})

	m, err := client.GetCookbookVersionManifest(ctx(), "nginx", "5.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.CookbookName != "nginx" {
		t.Errorf("CookbookName = %q, want %q", m.CookbookName, "nginx")
	}
	if m.Version != "5.1.0" {
		t.Errorf("Version = %q, want %q", m.Version, "5.1.0")
	}
	if len(m.Recipes) != 1 {
		t.Fatalf("Recipes length = %d, want 1", len(m.Recipes))
	}
	if m.Recipes[0].Path != "recipes/default.rb" {
		t.Errorf("Recipes[0].Path = %q, want %q", m.Recipes[0].Path, "recipes/default.rb")
	}
	if m.Recipes[0].Checksum != "abc123" {
		t.Errorf("Recipes[0].Checksum = %q, want %q", m.Recipes[0].Checksum, "abc123")
	}
	if len(m.Attributes) != 1 {
		t.Fatalf("Attributes length = %d, want 1", len(m.Attributes))
	}
	if len(m.RootFiles) != 1 {
		t.Fatalf("RootFiles length = %d, want 1", len(m.RootFiles))
	}
	if m.RootFiles[0].Name != "metadata.rb" {
		t.Errorf("RootFiles[0].Name = %q, want %q", m.RootFiles[0].Name, "metadata.rb")
	}
}

func TestGetCookbookVersionManifest_APIError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})

	_, err := client.GetCookbookVersionManifest(ctx(), "missing", "1.0.0")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

func TestGetCookbookVersionManifest_InvalidJSON(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not valid json`))
	})

	_, err := client.GetCookbookVersionManifest(ctx(), "bad", "1.0.0")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "unmarshalling cookbook version manifest") {
		t.Errorf("expected unmarshalling error, got: %v", err)
	}
}

func TestGetCookbookVersionManifest_EmptyManifest(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"cookbook_name":"empty","version":"0.1.0"}`))
	})

	m, err := client.GetCookbookVersionManifest(ctx(), "empty", "0.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.CookbookName != "empty" {
		t.Errorf("CookbookName = %q, want %q", m.CookbookName, "empty")
	}
	all := m.AllFiles()
	if len(all) != 0 {
		t.Errorf("AllFiles() = %d files, want 0 for empty manifest", len(all))
	}
}

// ---------------------------------------------------------------------------
// DownloadFileContent tests
// ---------------------------------------------------------------------------

func TestDownloadFileContent_Success(t *testing.T) {
	content := []byte("file content here")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	// DownloadFileContent uses client.httpClient directly with a plain
	// HTTP GET (no Chef API signing), so we need a client whose httpClient
	// can reach the test server.
	client, _ := NewClient(ClientConfig{
		ServerURL:     srv.URL + "/organizations/test",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		HTTPClient:    srv.Client(),
	})

	data, err := client.DownloadFileContent(ctx(), srv.URL+"/files/abc123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content = %q, want %q", data, content)
	}
}

func TestDownloadFileContent_ChecksumValid_SHA256(t *testing.T) {
	content := []byte("checksummed content")
	hash := sha256.Sum256(content)
	checksum := fmt.Sprintf("%x", hash) // 64 hex chars → SHA-256

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	client, _ := NewClient(ClientConfig{
		ServerURL:     srv.URL + "/organizations/test",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		HTTPClient:    srv.Client(),
	})

	data, err := client.DownloadFileContent(ctx(), srv.URL+"/files/abc", checksum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content = %q, want %q", data, content)
	}
}

// TestDownloadFileContent_ChecksumValid_MD5 verifies that MD5 checksums
// (32 hex chars) are validated correctly. Chef Server bookshelf uses MD5
// checksums in cookbook version manifests.
func TestDownloadFileContent_ChecksumValid_MD5(t *testing.T) {
	content := []byte("checksummed content for md5")
	hash := md5.Sum(content)
	checksum := fmt.Sprintf("%x", hash) // 32 hex chars → MD5

	if len(checksum) != 32 {
		t.Fatalf("expected 32 hex chars for MD5 checksum, got %d", len(checksum))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	client, _ := NewClient(ClientConfig{
		ServerURL:     srv.URL + "/organizations/test",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		HTTPClient:    srv.Client(),
	})

	data, err := client.DownloadFileContent(ctx(), srv.URL+"/files/abc", checksum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content = %q, want %q", data, content)
	}
}

// TestDownloadFileContent_ChecksumMismatch_MD5 verifies that an MD5
// checksum mismatch is correctly detected.
func TestDownloadFileContent_ChecksumMismatch_MD5(t *testing.T) {
	content := []byte("actual content for md5 mismatch")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	client, _ := NewClient(ClientConfig{
		ServerURL:     srv.URL + "/organizations/test",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		HTTPClient:    srv.Client(),
	})

	// 32 hex chars of zeros → MD5 that won't match
	_, err := client.DownloadFileContent(ctx(), srv.URL+"/files/abc", "00000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected checksum mismatch error, got: %v", err)
	}
}

func TestDownloadFileContent_ChecksumMismatch(t *testing.T) {
	content := []byte("actual content")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	client, _ := NewClient(ClientConfig{
		ServerURL:     srv.URL + "/organizations/test",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		HTTPClient:    srv.Client(),
	})

	_, err := client.DownloadFileContent(ctx(), srv.URL+"/files/abc", "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected checksum mismatch error, got: %v", err)
	}
}

func TestDownloadFileContent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("access denied"))
	}))
	defer srv.Close()

	client, _ := NewClient(ClientConfig{
		ServerURL:     srv.URL + "/organizations/test",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		HTTPClient:    srv.Client(),
	})

	_, err := client.DownloadFileContent(ctx(), srv.URL+"/files/abc", "")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 403 {
		t.Errorf("StatusCode = %d, want 403", apiErr.StatusCode)
	}
}

func TestDownloadFileContent_EmptyChecksum(t *testing.T) {
	content := []byte("no checksum validation")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	client, _ := NewClient(ClientConfig{
		ServerURL:     srv.URL + "/organizations/test",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		HTTPClient:    srv.Client(),
	})

	data, err := client.DownloadFileContent(ctx(), srv.URL+"/files/abc", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content = %q, want %q", data, content)
	}
}

func TestDownloadFileContent_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write nothing — empty body.
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, _ := NewClient(ClientConfig{
		ServerURL:     srv.URL + "/organizations/test",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		HTTPClient:    srv.Client(),
	})

	data, err := client.DownloadFileContent(ctx(), srv.URL+"/files/abc", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d bytes", len(data))
	}
}

// ---------------------------------------------------------------------------
// CookbookVersionManifest.AllFiles tests
// ---------------------------------------------------------------------------

func TestCookbookVersionManifest_AllFiles_Comprehensive(t *testing.T) {
	m := CookbookVersionManifest{
		Recipes:     []CookbookFileRef{{Path: "recipes/default.rb"}, {Path: "recipes/install.rb"}},
		Definitions: []CookbookFileRef{{Path: "definitions/mydef.rb"}},
		Libraries:   []CookbookFileRef{{Path: "libraries/helper.rb"}},
		Attributes:  []CookbookFileRef{{Path: "attributes/default.rb"}},
		Files:       []CookbookFileRef{{Path: "files/default/config.conf"}},
		Templates:   []CookbookFileRef{{Path: "templates/default/config.erb"}},
		Resources:   []CookbookFileRef{{Path: "resources/my_resource.rb"}},
		Providers:   []CookbookFileRef{{Path: "providers/my_provider.rb"}},
		RootFiles:   []CookbookFileRef{{Path: "metadata.rb"}, {Path: "README.md"}, {Path: "Berksfile"}},
	}

	all := m.AllFiles()
	if len(all) != 12 {
		t.Fatalf("AllFiles() returned %d files, want 12", len(all))
	}

	// Verify ordering: recipes, definitions, libraries, attributes, files,
	// templates, resources, providers, root_files.
	expectedPaths := []string{
		"recipes/default.rb", "recipes/install.rb",
		"definitions/mydef.rb",
		"libraries/helper.rb",
		"attributes/default.rb",
		"files/default/config.conf",
		"templates/default/config.erb",
		"resources/my_resource.rb",
		"providers/my_provider.rb",
		"metadata.rb", "README.md", "Berksfile",
	}
	for i, want := range expectedPaths {
		if all[i].Path != want {
			t.Errorf("AllFiles()[%d].Path = %q, want %q", i, all[i].Path, want)
		}
	}
}

func TestCookbookVersionManifest_AllFiles_EmptyCategories(t *testing.T) {
	m := CookbookVersionManifest{
		CookbookName: "minimal",
		Version:      "0.1.0",
		RootFiles:    []CookbookFileRef{{Path: "metadata.rb"}},
	}

	all := m.AllFiles()
	if len(all) != 1 {
		t.Fatalf("AllFiles() returned %d files, want 1", len(all))
	}
	if all[0].Path != "metadata.rb" {
		t.Errorf("AllFiles()[0].Path = %q, want %q", all[0].Path, "metadata.rb")
	}
}

func TestCookbookFileRef_Fields(t *testing.T) {
	ref := CookbookFileRef{
		Name:        "default.rb",
		Path:        "recipes/default.rb",
		Checksum:    "abc123def456",
		Specificity: "default",
		URL:         "https://bookshelf.example.com/files/abc123def456",
	}

	if ref.Name != "default.rb" {
		t.Errorf("Name = %q, want %q", ref.Name, "default.rb")
	}
	if ref.Path != "recipes/default.rb" {
		t.Errorf("Path = %q, want %q", ref.Path, "recipes/default.rb")
	}
	if ref.Checksum != "abc123def456" {
		t.Errorf("Checksum = %q, want %q", ref.Checksum, "abc123def456")
	}
	if ref.Specificity != "default" {
		t.Errorf("Specificity = %q, want %q", ref.Specificity, "default")
	}
	if ref.URL != "https://bookshelf.example.com/files/abc123def456" {
		t.Errorf("URL = %q, want expected URL", ref.URL)
	}
}

// ---------------------------------------------------------------------------
// Retry tests
// ---------------------------------------------------------------------------

func TestDoWithRetry_SuccessOnFirst(t *testing.T) {
	calls := 0
	result, err := DoWithRetry(ctx(), RetryConfig{MaxAttempts: 3, InitialWait: time.Millisecond}, func() (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestDoWithRetry_SuccessOnRetry(t *testing.T) {
	calls := 0
	result, err := DoWithRetry(ctx(), RetryConfig{MaxAttempts: 3, InitialWait: time.Millisecond}, func() (string, error) {
		calls++
		if calls < 3 {
			return "", &APIError{StatusCode: 500, Body: "error"}
		}
		return "recovered", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "recovered" {
		t.Errorf("expected 'recovered', got %q", result)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestDoWithRetry_ExhaustedRetries(t *testing.T) {
	calls := 0
	_, err := DoWithRetry(ctx(), RetryConfig{MaxAttempts: 2, InitialWait: time.Millisecond}, func() (string, error) {
		calls++
		return "", &APIError{StatusCode: 503, Body: "unavailable"}
	})
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if !strings.Contains(err.Error(), "max retries") {
		t.Errorf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestDoWithRetry_NonRetryableError(t *testing.T) {
	calls := 0
	_, err := DoWithRetry(ctx(), RetryConfig{MaxAttempts: 3, InitialWait: time.Millisecond}, func() (string, error) {
		calls++
		return "", &APIError{StatusCode: 401, Body: "unauthorized"}
	})
	if err == nil {
		t.Fatal("expected error for non-retryable status")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry for 401), got %d", calls)
	}
}

func TestDoWithRetry_NonAPIError(t *testing.T) {
	calls := 0
	_, err := DoWithRetry(ctx(), RetryConfig{MaxAttempts: 3, InitialWait: time.Millisecond}, func() (string, error) {
		calls++
		return "", fmt.Errorf("network error")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (non-APIError not retried), got %d", calls)
	}
}

func TestDoWithRetry_429Retried(t *testing.T) {
	calls := 0
	result, err := DoWithRetry(ctx(), RetryConfig{MaxAttempts: 3, InitialWait: time.Millisecond}, func() (string, error) {
		calls++
		if calls == 1 {
			return "", &APIError{StatusCode: 429, Body: "rate limited"}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestDoWithRetry_ContextCancelled(t *testing.T) {
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	calls := 0
	_, err := DoWithRetry(ctxCancel, RetryConfig{MaxAttempts: 5, InitialWait: time.Second}, func() (string, error) {
		calls++
		return "", &APIError{StatusCode: 500, Body: "error"}
	})
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
	// Should have been cancelled during the wait.
	if calls > 2 {
		t.Errorf("expected at most 2 calls with cancelled context, got %d", calls)
	}
}

func TestDoWithRetry_DefaultConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts 3, got %d", cfg.MaxAttempts)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("expected Multiplier 2.0, got %f", cfg.Multiplier)
	}
}

func TestDoWithRetry_ZeroConfigUsesDefaults(t *testing.T) {
	calls := 0
	result, err := DoWithRetry(ctx(), RetryConfig{}, func() (string, error) {
		calls++
		if calls < 3 {
			return "", &APIError{StatusCode: 500, Body: "error"}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
}

// ---------------------------------------------------------------------------
// NodeData helper tests
// ---------------------------------------------------------------------------

func TestNodeData_StringFields(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"name":             "web-01",
		"chef_environment": "production",
		"chef_version":     "17.10.0",
		"platform":         "ubuntu",
		"platform_version": "22.04",
		"platform_family":  "debian",
		"policy_name":      "webserver",
		"policy_group":     "prod",
	})

	tests := []struct {
		name string
		fn   func() string
		want string
	}{
		{"Name", nd.Name, "web-01"},
		{"ChefEnvironment", nd.ChefEnvironment, "production"},
		{"ChefVersion", nd.ChefVersion, "17.10.0"},
		{"Platform", nd.Platform, "ubuntu"},
		{"PlatformVersion", nd.PlatformVersion, "22.04"},
		{"PlatformFamily", nd.PlatformFamily, "debian"},
		{"PolicyName", nd.PolicyName, "webserver"},
		{"PolicyGroup", nd.PolicyGroup, "prod"},
	}
	for _, tt := range tests {
		if got := tt.fn(); got != tt.want {
			t.Errorf("%s: expected %q, got %q", tt.name, tt.want, got)
		}
	}
}

func TestNodeData_StringFields_Missing(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{})
	if nd.Name() != "" {
		t.Error("expected empty name for missing key")
	}
	if nd.Platform() != "" {
		t.Error("expected empty platform for missing key")
	}
}

func TestNodeData_StringFields_WrongType(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"name": 12345, // not a string
	})
	if nd.Name() != "" {
		t.Error("expected empty string for wrong type")
	}
}

func TestNodeData_IsPolicyfileNode(t *testing.T) {
	policyNode := NewNodeData(map[string]interface{}{
		"policy_name":  "web",
		"policy_group": "prod",
	})
	if !policyNode.IsPolicyfileNode() {
		t.Error("expected IsPolicyfileNode to be true")
	}

	classicNode := NewNodeData(map[string]interface{}{})
	if classicNode.IsPolicyfileNode() {
		t.Error("expected IsPolicyfileNode to be false for classic node")
	}

	partialPolicy := NewNodeData(map[string]interface{}{
		"policy_name": "web",
	})
	if partialPolicy.IsPolicyfileNode() {
		t.Error("expected IsPolicyfileNode to be false when only policy_name is set")
	}
}

func TestNodeData_OhaiTime(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"ohai_time": 1718444400.123456,
	})
	if nd.OhaiTime() != 1718444400.123456 {
		t.Errorf("unexpected ohai_time: %f", nd.OhaiTime())
	}
}

func TestNodeData_OhaiTimeAsTime(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"ohai_time": 1718444400.0,
	})
	tm := nd.OhaiTimeAsTime()
	if tm.Unix() != 1718444400 {
		t.Errorf("unexpected time: %v", tm)
	}
}

func TestNodeData_OhaiTimeAsTime_Zero(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{})
	tm := nd.OhaiTimeAsTime()
	if !tm.IsZero() {
		t.Error("expected zero time for missing ohai_time")
	}
}

func TestNodeData_IsStale(t *testing.T) {
	recentTime := float64(time.Now().Add(-1 * time.Hour).Unix())
	recent := NewNodeData(map[string]interface{}{
		"ohai_time": recentTime,
	})
	if recent.IsStale(7 * 24 * time.Hour) {
		t.Error("recent node should not be stale")
	}

	oldTime := float64(time.Now().Add(-30 * 24 * time.Hour).Unix())
	old := NewNodeData(map[string]interface{}{
		"ohai_time": oldTime,
	})
	if !old.IsStale(7 * 24 * time.Hour) {
		t.Error("old node should be stale")
	}
}

func TestNodeData_IsStale_MissingOhaiTime(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{})
	if !nd.IsStale(7 * 24 * time.Hour) {
		t.Error("node with no ohai_time should be treated as stale")
	}
}

func TestNodeData_Cookbooks(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"cookbooks": map[string]interface{}{
			"nginx": map[string]interface{}{"version": "5.1.0"},
			"apt":   map[string]interface{}{"version": "7.4.0"},
		},
	})
	cbs := nd.Cookbooks()
	if len(cbs) != 2 {
		t.Errorf("expected 2 cookbooks, got %d", len(cbs))
	}
	if cbs["nginx"]["version"] != "5.1.0" {
		t.Errorf("unexpected nginx version: %v", cbs["nginx"]["version"])
	}
}

func TestNodeData_Cookbooks_Missing(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{})
	if nd.Cookbooks() != nil {
		t.Error("expected nil for missing cookbooks")
	}
}

func TestNodeData_Cookbooks_WrongType(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"cookbooks": "not a map",
	})
	if nd.Cookbooks() != nil {
		t.Error("expected nil for wrong type cookbooks")
	}
}

func TestNodeData_CookbookVersions(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"cookbooks": map[string]interface{}{
			"nginx": map[string]interface{}{"version": "5.1.0"},
			"apt":   map[string]interface{}{"version": "7.4.0"},
			"base":  map[string]interface{}{}, // no version key
		},
	})
	versions := nd.CookbookVersions()
	if len(versions) != 2 {
		t.Errorf("expected 2 versioned cookbooks, got %d", len(versions))
	}
	if versions["nginx"] != "5.1.0" {
		t.Errorf("unexpected nginx version: %q", versions["nginx"])
	}
	if versions["apt"] != "7.4.0" {
		t.Errorf("unexpected apt version: %q", versions["apt"])
	}
}

func TestNodeData_CookbookVersions_NoCookbooks(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{})
	if nd.CookbookVersions() != nil {
		t.Error("expected nil for no cookbooks")
	}
}

func TestNodeData_RunList(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"run_list": []interface{}{"role[base]", "recipe[nginx::default]"},
	})
	rl := nd.RunList()
	if len(rl) != 2 {
		t.Errorf("expected 2 run_list entries, got %d", len(rl))
	}
	if rl[0] != "role[base]" {
		t.Errorf("unexpected run_list[0]: %q", rl[0])
	}
}

func TestNodeData_RunList_Missing(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{})
	if nd.RunList() != nil {
		t.Error("expected nil for missing run_list")
	}
}

func TestNodeData_RunList_WrongType(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"run_list": "not an array",
	})
	if nd.RunList() != nil {
		t.Error("expected nil for wrong type run_list")
	}
}

func TestNodeData_Roles(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"roles": []interface{}{"base", "webserver"},
	})
	roles := nd.Roles()
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}

func TestNodeData_Roles_Missing(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{})
	if nd.Roles() != nil {
		t.Error("expected nil for missing roles")
	}
}

func TestNodeData_Tags(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"tags": []interface{}{"prepare", "upgrade"},
	})
	tags := nd.Tags()
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d: %v", len(tags), tags)
	}
}

// TestNodeData_Tags_Missing verifies the spec invariant that a node which
// never had tags set (Chef returns null / the key is absent) coalesces to an
// empty, non-nil slice — never nil. "Collected, no tags" is [], never null.
func TestNodeData_Tags_Missing(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{})
	tags := nd.Tags()
	if tags == nil {
		t.Fatal("expected non-nil empty slice for missing tags, got nil")
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d: %v", len(tags), tags)
	}
}

// TestNodeData_Tags_Null verifies an explicit JSON null coalesces to [].
func TestNodeData_Tags_Null(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{"tags": nil})
	tags := nd.Tags()
	if tags == nil {
		t.Fatal("expected non-nil empty slice for null tags, got nil")
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d: %v", len(tags), tags)
	}
}

// TestNodeData_Tags_Dedup verifies duplicate tags are removed (order not
// significant) per the spec's de-duplication invariant.
func TestNodeData_Tags_Dedup(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"tags": []interface{}{"pci", "prepare", "pci", "prepare", "eu-west"},
	})
	tags := nd.Tags()
	if len(tags) != 3 {
		t.Errorf("expected 3 de-duplicated tags, got %d: %v", len(tags), tags)
	}
	seen := map[string]int{}
	for _, tag := range tags {
		seen[tag]++
	}
	for tag, n := range seen {
		if n != 1 {
			t.Errorf("tag %q appears %d times, expected 1", tag, n)
		}
	}
}

// TestNodeData_Tags_CaseSensitive verifies tags are matched/stored exactly as
// Chef returns them — no lowercasing or trimming; case variants are distinct.
func TestNodeData_Tags_CaseSensitive(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"tags": []interface{}{"Prepare", "prepare"},
	})
	tags := nd.Tags()
	if len(tags) != 2 {
		t.Errorf("expected 2 case-distinct tags, got %d: %v", len(tags), tags)
	}
}

func TestNodeData_Filesystem(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"filesystem": map[string]interface{}{
			"/": map[string]interface{}{"kb_available": float64(204800)},
		},
	})
	fs := nd.Filesystem()
	if fs == nil {
		t.Fatal("expected non-nil filesystem")
	}
}

func TestNodeData_Filesystem_Missing(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{})
	if nd.Filesystem() != nil {
		t.Error("expected nil for missing filesystem")
	}
}

func TestNodeData_FreeDiskMB_DirectRoot(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"filesystem": map[string]interface{}{
			"/": map[string]interface{}{"kb_available": float64(204800)},
		},
	})
	mb := nd.FreeDiskMB()
	if mb != 200 { // 204800 / 1024
		t.Errorf("expected 200 MB, got %d", mb)
	}
}

func TestNodeData_FreeDiskMB_ByMountpoint(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"filesystem": map[string]interface{}{
			"by_mountpoint": map[string]interface{}{
				"/": map[string]interface{}{"kb_available": float64(1048576)},
			},
		},
	})
	mb := nd.FreeDiskMB()
	if mb != 1024 { // 1048576 / 1024
		t.Errorf("expected 1024 MB, got %d", mb)
	}
}

func TestNodeData_FreeDiskMB_StringValue(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"filesystem": map[string]interface{}{
			"/": map[string]interface{}{"kb_available": "204800"},
		},
	})
	mb := nd.FreeDiskMB()
	if mb != 200 {
		t.Errorf("expected 200 MB from string value, got %d", mb)
	}
}

func TestNodeData_FreeDiskMB_NoFilesystem(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{})
	if nd.FreeDiskMB() != -1 {
		t.Error("expected -1 for missing filesystem")
	}
}

func TestNodeData_FreeDiskMB_NoRootMount(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"filesystem": map[string]interface{}{
			"/boot": map[string]interface{}{"kb_available": float64(512000)},
		},
	})
	if nd.FreeDiskMB() != -1 {
		t.Error("expected -1 when root mount is missing")
	}
}

func TestNodeData_FreeDiskMB_NoKBAvailable(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"filesystem": map[string]interface{}{
			"/": map[string]interface{}{"mount": "/"},
		},
	})
	if nd.FreeDiskMB() != -1 {
		t.Error("expected -1 when kb_available is missing")
	}
}

// ---------------------------------------------------------------------------
// Integration-style test: full search flow
// ---------------------------------------------------------------------------

func TestFullSearchFlow(t *testing.T) {
	// Simulate a Chef server returning realistic node data.
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		resp := SearchResult{
			Total: 3,
			Start: 0,
			Rows: []SearchResultRow{
				{
					URL: "https://chef/nodes/web-01",
					Data: map[string]interface{}{
						"name":             "web-01",
						"chef_environment": "production",
						"chef_version":     "17.10.0",
						"platform":         "ubuntu",
						"platform_version": "22.04",
						"platform_family":  "debian",
						"cookbooks": map[string]interface{}{
							"nginx": map[string]interface{}{"version": "5.1.0"},
							"base":  map[string]interface{}{"version": "1.3.2"},
						},
						"run_list":     []interface{}{"role[base]", "recipe[nginx::default]"},
						"roles":        []interface{}{"base", "webserver"},
						"ohai_time":    float64(time.Now().Unix()),
						"policy_name":  nil,
						"policy_group": nil,
						"filesystem": map[string]interface{}{
							"/": map[string]interface{}{"kb_available": float64(5242880)},
						},
					},
				},
				{
					URL: "https://chef/nodes/app-01",
					Data: map[string]interface{}{
						"name":             "app-01",
						"chef_environment": "staging",
						"chef_version":     "18.5.0",
						"platform":         "centos",
						"platform_version": "7",
						"platform_family":  "rhel",
						"cookbooks": map[string]interface{}{
							"java": map[string]interface{}{"version": "8.0.0"},
						},
						"run_list":     []interface{}{"recipe[java::default]"},
						"roles":        []interface{}{},
						"ohai_time":    float64(time.Now().Add(-30 * 24 * time.Hour).Unix()),
						"policy_name":  "appserver",
						"policy_group": "staging",
					},
				},
				{
					URL: "https://chef/nodes/db-01",
					Data: map[string]interface{}{
						"name":             "db-01",
						"chef_environment": "production",
						"chef_version":     "16.0.0",
						"platform":         "rhel",
						"platform_version": "8",
						"platform_family":  "rhel",
						"ohai_time":        float64(0), // no ohai_time
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	rows, err := client.CollectAllNodes(ctx(), 1000, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Verify node 1: classic node with recent ohai_time
	web := NewNodeData(rows[0].Data)
	if web.Name() != "web-01" {
		t.Errorf("expected 'web-01', got %q", web.Name())
	}
	if web.IsPolicyfileNode() {
		t.Error("web-01 should not be a Policyfile node")
	}
	if web.IsStale(7 * 24 * time.Hour) {
		t.Error("web-01 should not be stale")
	}
	versions := web.CookbookVersions()
	if versions["nginx"] != "5.1.0" {
		t.Errorf("web-01 nginx version: %q", versions["nginx"])
	}
	if web.FreeDiskMB() != 5120 {
		t.Errorf("web-01 free disk: %d MB", web.FreeDiskMB())
	}

	// Verify node 2: Policyfile node, stale
	app := NewNodeData(rows[1].Data)
	if !app.IsPolicyfileNode() {
		t.Error("app-01 should be a Policyfile node")
	}
	if app.PolicyName() != "appserver" {
		t.Errorf("app-01 policy_name: %q", app.PolicyName())
	}
	if !app.IsStale(7 * 24 * time.Hour) {
		t.Error("app-01 should be stale (30 days old)")
	}

	// Verify node 3: no ohai_time → stale
	db := NewNodeData(rows[2].Data)
	if !db.IsStale(7 * 24 * time.Hour) {
		t.Error("db-01 with ohai_time 0 should be treated as stale")
	}
	if db.FreeDiskMB() != -1 {
		t.Error("db-01 with no filesystem should return -1")
	}
}

// ---------------------------------------------------------------------------
// parsePrivateKey edge cases
// ---------------------------------------------------------------------------

func TestParsePrivateKey_PKCS8NonRSA(t *testing.T) {
	// Create a valid PEM block with PRIVATE KEY type but garbage content
	// that would parse as PKCS#8 but not RSA. We'll use an invalid DER
	// to trigger the parse error path.
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: []byte("not-valid-der")}
	pemData := pem.EncodeToMemory(block)

	_, err := parsePrivateKey(pemData)
	if err == nil {
		t.Fatal("expected error for invalid PKCS#8 data")
	}
}

// ---------------------------------------------------------------------------
// context helper
// ---------------------------------------------------------------------------

func ctx() context.Context {
	return context.Background()
}

// ---------------------------------------------------------------------------
// PlatformCaption tests
// ---------------------------------------------------------------------------

func TestNodeData_PlatformCaption_Windows(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"platform":               "windows",
		"platform_family":        "windows",
		"kernel_os_info_caption": "Microsoft Windows Server 2022 Datacenter",
	})
	got := nd.PlatformCaption()
	want := "Microsoft Windows Server 2022 Datacenter"
	if got != want {
		t.Errorf("PlatformCaption() = %q, want %q", got, want)
	}
}

func TestNodeData_PlatformCaption_Ubuntu(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"platform":        "ubuntu",
		"platform_family": "debian",
		"lsb_description": "Ubuntu 24.04.4 LTS",
	})
	got := nd.PlatformCaption()
	want := "Ubuntu 24.04.4 LTS"
	if got != want {
		t.Errorf("PlatformCaption() = %q, want %q", got, want)
	}
}

func TestNodeData_PlatformCaption_Empty(t *testing.T) {
	nd := NewNodeData(map[string]interface{}{
		"platform":        "redhat",
		"platform_family": "rhel",
	})
	got := nd.PlatformCaption()
	if got != "" {
		t.Errorf("PlatformCaption() = %q, want empty", got)
	}
}

func TestNodeData_PlatformCaption_WindowsFamilyMissing(t *testing.T) {
	// Windows node with missing platform_family but kernel caption present.
	nd := NewNodeData(map[string]interface{}{
		"platform":               "windows",
		"kernel_os_info_caption": "Microsoft Windows Server 2019 Standard",
	})
	got := nd.PlatformCaption()
	want := "Microsoft Windows Server 2019 Standard"
	if got != want {
		t.Errorf("PlatformCaption() = %q, want %q", got, want)
	}
}

func TestNodeData_PlatformCaption_PreferWindowsCaption(t *testing.T) {
	// Windows node with both fields — should prefer kernel caption.
	nd := NewNodeData(map[string]interface{}{
		"platform":               "windows",
		"platform_family":        "windows",
		"kernel_os_info_caption": "Microsoft Windows 11 Enterprise",
		"lsb_description":        "should not be used",
	})
	got := nd.PlatformCaption()
	want := "Microsoft Windows 11 Enterprise"
	if got != want {
		t.Errorf("PlatformCaption() = %q, want %q", got, want)
	}
}

func TestNodeData_PlatformCaption_FallbackToKernelCaption(t *testing.T) {
	// Non-windows node with kernel caption but no LSB — still returns it.
	nd := NewNodeData(map[string]interface{}{
		"platform":               "unknown",
		"platform_family":        "",
		"kernel_os_info_caption": "Some OS Caption",
	})
	got := nd.PlatformCaption()
	want := "Some OS Caption"
	if got != want {
		t.Errorf("PlatformCaption() = %q, want %q", got, want)
	}
}

func TestNodeData_PlatformCaption_UTF16BOMStripped(t *testing.T) {
	// Caption with UTF-16 BOM bytes (0xFF 0xFE) that didn't get transcoded.
	nd := NewNodeData(map[string]interface{}{
		"platform":               "windows",
		"platform_family":        "windows",
		"kernel_os_info_caption": "\xff\xfeMicrosoft Windows Server 2016 Standard",
	})
	got := nd.PlatformCaption()
	want := "Microsoft Windows Server 2016 Standard"
	if got != want {
		t.Errorf("PlatformCaption() = %q, want %q", got, want)
	}
}

func TestNodeData_PlatformCaption_InvalidUTF8Stripped(t *testing.T) {
	// Caption with scattered invalid bytes.
	nd := NewNodeData(map[string]interface{}{
		"platform":               "windows",
		"platform_family":        "windows",
		"kernel_os_info_caption": "Microsoft\x80 Windows\xfe Server",
	})
	got := nd.PlatformCaption()
	want := "Microsoft Windows Server"
	if got != want {
		t.Errorf("PlatformCaption() = %q, want %q", got, want)
	}
}

func TestNodeData_PlatformCaption_ValidUTF8Unchanged(t *testing.T) {
	// Valid UTF-8 with non-ASCII characters passes through.
	nd := NewNodeData(map[string]interface{}{
		"platform":               "windows",
		"platform_family":        "windows",
		"kernel_os_info_caption": "Microsoft Windows Server 2022 Données",
	})
	got := nd.PlatformCaption()
	want := "Microsoft Windows Server 2022 Données"
	if got != want {
		t.Errorf("PlatformCaption() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// GetRolesConcurrent tests
// ---------------------------------------------------------------------------

func TestGetRolesConcurrent_ReturnsAllRolesInRequestOrder(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/organizations/testorg/roles/")
		_ = json.NewEncoder(w).Encode(RoleDetail{
			Name:    name,
			RunList: []string{"recipe[" + name + "]"},
		})
	})

	names := []string{"alpha", "bravo", "charlie", "delta", "echo"}
	roles, errs := client.GetRolesConcurrent(ctx(), names, 3)

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(roles) != len(names) {
		t.Fatalf("expected %d roles, got %d", len(names), len(roles))
	}
	// Order must match the requested name order so the dependency graph is
	// built deterministically regardless of completion order.
	for i, want := range names {
		if roles[i].Name != want {
			t.Errorf("roles[%d].Name = %q, want %q", i, roles[i].Name, want)
		}
	}
}

func TestGetRolesConcurrent_SkipsFailedRolesAndReportsErrors(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/organizations/testorg/roles/")
		if name == "bravo" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(RoleDetail{Name: name})
	})

	names := []string{"alpha", "bravo", "charlie"}
	roles, errs := client.GetRolesConcurrent(ctx(), names, 2)

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}
	// A failing role is skipped, and the surviving roles keep request order.
	for i, want := range []string{"alpha", "charlie"} {
		if roles[i].Name != want {
			t.Errorf("roles[%d].Name = %q, want %q", i, roles[i].Name, want)
		}
	}
	if !strings.Contains(errs[0].Error(), "bravo") {
		t.Errorf("error should name the failing role, got %v", errs[0])
	}
}

func TestGetRolesConcurrent_RespectsConcurrencyLimit(t *testing.T) {
	var inFlight, maxInFlight int64
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			old := atomic.LoadInt64(&maxInFlight)
			if cur <= old || atomic.CompareAndSwapInt64(&maxInFlight, old, cur) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
		_ = json.NewEncoder(w).Encode(RoleDetail{
			Name: strings.TrimPrefix(r.URL.Path, "/organizations/testorg/roles/"),
		})
	})

	names := make([]string, 20)
	for i := range names {
		names[i] = fmt.Sprintf("role%02d", i)
	}

	const limit = 4
	roles, errs := client.GetRolesConcurrent(ctx(), names, limit)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(roles) != len(names) {
		t.Fatalf("expected %d roles, got %d", len(names), len(roles))
	}
	if got := atomic.LoadInt64(&maxInFlight); got > limit {
		t.Errorf("max concurrent requests = %d, want <= %d", got, limit)
	}
}

func TestGetRolesConcurrent_EmptyInput(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request should be made for an empty role list")
		w.WriteHeader(http.StatusInternalServerError)
	})

	roles, errs := client.GetRolesConcurrent(ctx(), nil, 5)
	if len(roles) != 0 || len(errs) != 0 {
		t.Errorf("expected no roles and no errors, got %d roles / %d errors", len(roles), len(errs))
	}
}

func TestGetRolesConcurrent_ZeroConcurrencyDefaultsToSerial(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(RoleDetail{
			Name: strings.TrimPrefix(r.URL.Path, "/organizations/testorg/roles/"),
		})
	})

	roles, errs := client.GetRolesConcurrent(ctx(), []string{"a", "b"}, 0)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}
}

func TestGetRolesConcurrent_ContextCancellation(t *testing.T) {
	cancelCtx, cancel := context.WithCancel(context.Background())
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		cancel()
		time.Sleep(5 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(RoleDetail{
			Name: strings.TrimPrefix(r.URL.Path, "/organizations/testorg/roles/"),
		})
	})

	names := make([]string, 50)
	for i := range names {
		names[i] = fmt.Sprintf("role%02d", i)
	}

	// Must return rather than hang; a cancelled context short-circuits the
	// remaining fetches instead of issuing 50 doomed requests.
	roles, _ := client.GetRolesConcurrent(cancelCtx, names, 2)
	if len(roles) == len(names) {
		t.Errorf("expected cancellation to skip some roles, got all %d", len(roles))
	}
}

// ---------------------------------------------------------------------------
// HTTP transport timeout tests
// ---------------------------------------------------------------------------

func TestNewClient_DefaultHTTPClientHasTimeouts(t *testing.T) {
	c, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	hc := c.httpClient
	if hc == http.DefaultClient {
		t.Fatal("client must not use http.DefaultClient — it has no timeout, so a stalled Chef server hangs a collection run forever")
	}
	if hc.Timeout <= 0 {
		t.Error("http client must set an overall Timeout")
	}

	tr, ok := hc.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", hc.Transport)
	}
	if tr.TLSHandshakeTimeout <= 0 {
		t.Error("TLSHandshakeTimeout must be set")
	}
	if tr.ResponseHeaderTimeout <= 0 {
		t.Error("ResponseHeaderTimeout must be set — it is what detects a stalled server")
	}
	if tr.MaxIdleConnsPerHost < 2 {
		t.Errorf("MaxIdleConnsPerHost = %d; the default of 2 throttles concurrent role fetching", tr.MaxIdleConnsPerHost)
	}
}

func TestNewClient_InsecureClientAlsoHasTimeouts(t *testing.T) {
	sslVerify := false
	c, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		SSLVerify:     &sslVerify,
		WarnFunc:      func(string) {},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if c.httpClient.Timeout <= 0 {
		t.Error("insecure client must also set an overall Timeout")
	}
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.httpClient.Transport)
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify must be preserved when ssl_verify is false")
	}
	if tr.ResponseHeaderTimeout <= 0 {
		t.Error("ResponseHeaderTimeout must be set on the insecure client too")
	}
}

func TestNewClient_RespectsSuppliedHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 3 * time.Second}
	c, err := NewClient(ClientConfig{
		ServerURL:     "https://chef.example.com/organizations/myorg",
		ClientName:    "test",
		PrivateKeyPEM: generateTestKey(t),
		HTTPClient:    custom,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.httpClient != custom {
		t.Error("an explicitly supplied HTTPClient must be used unchanged")
	}
}

func TestNodeSearchAttributes_FilesystemIsNotNarrowed(t *testing.T) {
	// The filesystem attribute must be requested as a whole subtree.
	//
	// Narrowing it to ["filesystem","by_pair"] (v2.18.6) destroyed filesystem
	// data for every Windows node: 55,488 of 55,489 lost the attribute — and
	// their disk verdict with it — on the first post-deploy collection, with
	// no error and no log line.
	//
	// The sections Ohai emits are not the same on every platform. Windows
	// delivers by_mountpoint keyed by drive letter and no by_pair; see the
	// fixture in TestHandleNodeDisks_HappyPath_Windows (internal/webapi),
	// which has encoded that shape since long before the narrowing. Any
	// narrowed sub-path silently yields nothing wherever that section is
	// absent.
	//
	// Before narrowing again, validate the shape distribution across the whole
	// fleet from node_snapshots — see the census requirement in
	// journeys/service-collection.md. A single node, or a lab
	// running current Chef, is not evidence about a heterogeneous estate.
	attrs := NodeSearchAttributes()

	path, ok := attrs["filesystem"]
	if !ok {
		t.Fatal("filesystem attribute must be collected")
	}
	if len(path) != 1 || path[0] != "filesystem" {
		t.Errorf("filesystem path = %v, want [filesystem]; a narrowed sub-path silently drops every platform whose Ohai lacks that section", path)
	}
}
