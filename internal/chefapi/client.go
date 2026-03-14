// Package chefapi provides a native Go client for the Chef Infra Server API.
// It implements RSA request signing (protocol version 1.3, SHA-256) without
// external authentication libraries, and provides methods for partial search
// (node collection) and pagination.
package chefapi

import (
	"bytes"
	"context"
	"crypto"
	"crypto/md5"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Client configuration
// ---------------------------------------------------------------------------

// ClientConfig holds the parameters needed to construct a Chef API Client.
type ClientConfig struct {
	// ServerURL is the base URL of the Chef Infra Server including the
	// organisation path, e.g. "https://chef.example.com/organizations/myorg".
	ServerURL string

	// ClientName is the name of the API client to authenticate as. This
	// value is sent in the X-Ops-UserId header.
	ClientName string

	// PrivateKeyPEM is the RSA private key in PEM format used to sign
	// requests. The caller is responsible for zeroing this slice after
	// the Client is constructed.
	PrivateKeyPEM []byte

	// OrgName is used in the User-Agent header for identification.
	OrgName string

	// AppVersion is used in the User-Agent header. Defaults to "dev".
	AppVersion string

	// SSLVerify controls whether the client verifies the server's TLS
	// certificate chain and hostname. When set to false, the client
	// accepts any certificate presented by the server. This is useful
	// for development or self-signed certificate environments but should
	// NOT be used in production. Defaults to true (verify).
	SSLVerify *bool

	// HTTPClient is an optional *http.Client to use. If nil,
	// a default client is created. When SSLVerify is false and no
	// HTTPClient is provided, the default client is configured to
	// skip TLS verification.
	HTTPClient *http.Client
}

// Client is a Chef Infra Server API client that signs all requests using
// the RSA v1.3 signing protocol.
type Client struct {
	serverURL  *url.URL
	clientName string
	privateKey *rsa.PrivateKey
	userAgent  string
	httpClient *http.Client
}

// NewClient creates a new Chef API Client from the given configuration.
// It parses the PEM-encoded private key and validates the server URL.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("chefapi: ServerURL is required")
	}
	if cfg.ClientName == "" {
		return nil, fmt.Errorf("chefapi: ClientName is required")
	}
	if len(cfg.PrivateKeyPEM) == 0 {
		return nil, fmt.Errorf("chefapi: PrivateKeyPEM is required")
	}

	u, err := url.Parse(cfg.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("chefapi: invalid ServerURL: %w", err)
	}
	// Ensure trailing slash is stripped for consistent path joining.
	u.Path = strings.TrimRight(u.Path, "/")

	key, err := parsePrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("chefapi: %w", err)
	}

	appVersion := cfg.AppVersion
	if appVersion == "" {
		appVersion = "dev"
	}
	orgName := cfg.OrgName
	if orgName == "" {
		orgName = "unknown"
	}
	ua := fmt.Sprintf("chef-migration-metrics/%s (org:%s)", appVersion, orgName)

	sslVerify := true
	if cfg.SSLVerify != nil {
		sslVerify = *cfg.SSLVerify
	}

	hc := cfg.HTTPClient
	if hc == nil {
		if !sslVerify {
			hc = &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true, // #nosec G402 -- user explicitly opted out of verification
					},
				},
			}
		} else {
			hc = http.DefaultClient
		}
	}

	return &Client{
		serverURL:  u,
		clientName: cfg.ClientName,
		privateKey: key,
		userAgent:  ua,
		httpClient: hc,
	}, nil
}

// ---------------------------------------------------------------------------
// RSA private key parsing
// ---------------------------------------------------------------------------

// parsePrivateKey decodes a PEM-encoded RSA private key. It supports both
// PKCS#1 (RSA PRIVATE KEY) and PKCS#8 (PRIVATE KEY) formats.
func parsePrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from private key")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS#8 private key: %w", err)
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS#8 key is not RSA (got %T)", parsed)
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q (expected RSA PRIVATE KEY or PRIVATE KEY)", block.Type)
	}
}

// ---------------------------------------------------------------------------
// Request signing (v1.3, SHA-256)
// ---------------------------------------------------------------------------

// signRequest adds the Chef API authentication headers to an *http.Request.
// It implements signing protocol version 1.3 with SHA-256.
func (c *Client) signRequest(req *http.Request, body []byte) error {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	// Content hash — SHA-256 of the request body, base64-encoded.
	bodyHash := sha256.Sum256(body)
	contentHash := base64.StdEncoding.EncodeToString(bodyHash[:])

	// Path for signing — use the raw path from the URL.
	path := req.URL.Path
	if path == "" {
		path = "/"
	}

	// Build the canonical header string.
	canonical := strings.Join([]string{
		"Method:" + req.Method,
		"Path:" + path,
		"X-Ops-Content-Hash:" + contentHash,
		"X-Ops-Sign:version=1.3",
		"X-Ops-Timestamp:" + timestamp,
		"X-Ops-UserId:" + c.clientName,
		"X-Ops-Server-API-Version:1",
	}, "\n")

	// Sign the canonical string with RSA-SHA256.
	hashed := sha256.Sum256([]byte(canonical))
	sig, err := rsa.SignPKCS1v15(nil, c.privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return fmt.Errorf("chefapi: RSA signing failed: %w", err)
	}

	// Base64-encode and split into 60-character segments.
	encoded := base64.StdEncoding.EncodeToString(sig)
	authHeaders := splitString(encoded, 60)

	// Set headers.
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-Chef-Version", "17.0.0")
	req.Header.Set("X-Ops-Sign", "version=1.3")
	req.Header.Set("X-Ops-Timestamp", timestamp)
	req.Header.Set("X-Ops-UserId", c.clientName)
	req.Header.Set("X-Ops-Content-Hash", contentHash)
	req.Header.Set("X-Ops-Server-API-Version", "1")

	for i, segment := range authHeaders {
		req.Header.Set(fmt.Sprintf("X-Ops-Authorization-%d", i+1), segment)
	}

	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	return nil
}

// splitString splits s into chunks of at most n characters.
func splitString(s string, n int) []string {
	if n <= 0 {
		return []string{s}
	}
	chunks := make([]string, 0, (len(s)+n-1)/n)
	for len(s) > 0 {
		end := n
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[:end])
		s = s[end:]
	}
	return chunks
}

// ---------------------------------------------------------------------------
// Low-level HTTP helpers
// ---------------------------------------------------------------------------

// doRequest builds, signs, and executes an HTTP request. It returns the
// response body bytes. Non-2xx status codes are returned as *APIError.
func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	u := *c.serverURL

	// Parse query parameters out of the path string so they end up in
	// u.RawQuery rather than being appended to u.Path (which would
	// double-encode them).
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		u.Path = u.Path + path[:idx]
		u.RawQuery = path[idx+1:]
	} else {
		u.Path = u.Path + path
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("chefapi: creating request: %w", err)
	}

	if body == nil {
		body = []byte{}
	}
	if err := c.signRequest(req, body); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chefapi: executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("chefapi: reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return respBody, &APIError{
			StatusCode: resp.StatusCode,
			Method:     method,
			Path:       path,
			Body:       string(respBody),
		}
	}

	return respBody, nil
}

// APIError represents a non-2xx HTTP response from the Chef server.
type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("chefapi: %s %s returned %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// IsRetryable returns true if the error represents a status code that may
// succeed on retry (429, 5xx).
func (e *APIError) IsRetryable() bool {
	return e.StatusCode == 429 || e.StatusCode >= 500
}

// IsNotFound returns true if the error is a 404 response.
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == 404
}

// ---------------------------------------------------------------------------
// Partial Search
// ---------------------------------------------------------------------------

// SearchResult represents the response from a Chef partial search request.
type SearchResult struct {
	Total int               `json:"total"`
	Start int               `json:"start"`
	Rows  []SearchResultRow `json:"rows"`
}

// SearchResultRow is a single row in a search response.
type SearchResultRow struct {
	URL  string                 `json:"url"`
	Data map[string]interface{} `json:"data"`
}

// PartialSearchQuery maps friendly attribute names to their Chef attribute
// paths. For example: {"chef_version": ["automatic", "chef_packages", "chef", "version"]}.
type PartialSearchQuery map[string][]string

// PartialSearch executes a single partial search request against the given
// index with pagination parameters.
func (c *Client) PartialSearch(ctx context.Context, index, query string, rows, start int, attributes PartialSearchQuery) (*SearchResult, error) {
	path := fmt.Sprintf("/search/%s?q=%s&rows=%d&start=%d",
		url.PathEscape(index),
		url.QueryEscape(query),
		rows,
		start,
	)

	body, err := json.Marshal(attributes)
	if err != nil {
		return nil, fmt.Errorf("chefapi: marshalling search body: %w", err)
	}

	respBody, err := c.doRequest(ctx, "POST", path, body)
	if err != nil {
		return nil, err
	}

	var result SearchResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("chefapi: unmarshalling search response: %w", err)
	}

	return &result, nil
}

// ---------------------------------------------------------------------------
// Node collection — standard partial search query
// ---------------------------------------------------------------------------

// NodeSearchAttributes returns the standard PartialSearchQuery for node
// collection as defined in the Chef API specification.
func NodeSearchAttributes() PartialSearchQuery {
	// Partial search paths navigate the merged attribute namespace, not
	// the raw storage structure. Attributes stored under "automatic" by
	// Ohai (platform, cookbooks, filesystem, etc.) are accessed directly
	// by name — the "automatic" prefix must NOT appear in the path.
	return PartialSearchQuery{
		"name":             {"name"},
		"chef_environment": {"chef_environment"},
		"chef_version":     {"chef_packages", "chef", "version"},
		"platform":         {"platform"},
		"platform_version": {"platform_version"},
		"platform_family":  {"platform_family"},
		"filesystem":       {"filesystem"},
		"cookbooks":        {"cookbooks"},
		"run_list":         {"run_list"},
		"roles":            {"roles"},
		"policy_name":      {"policy_name"},
		"policy_group":     {"policy_group"},
		"ohai_time":        {"ohai_time"},
	}
}

// NodeSearchAttributesWithExtra returns the standard node partial search
// query merged with additional keys. Extra keys that collide with standard
// keys are silently ignored so callers cannot accidentally overwrite core
// fields. This is used to request CMDB ownership attributes (e.g.
// "itil.cmdb.node" → ["itil", "cmdb", "node"]) alongside the standard set.
func NodeSearchAttributesWithExtra(extra map[string][]string) PartialSearchQuery {
	attrs := NodeSearchAttributes()
	for key, path := range extra {
		if _, exists := attrs[key]; !exists {
			attrs[key] = path
		}
	}
	return attrs
}

// CollectAllNodes performs a paginated partial search to collect all nodes
// from the Chef server. It fetches pages sequentially, collecting all rows
// into a single slice. For concurrent page fetching, use CollectAllNodesConcurrent.
//
// The pageSize parameter controls the number of nodes per request (recommended: 1000).
// The optional extraAttrs parameter supplies additional partial-search keys
// (e.g. CMDB ownership attributes) that are merged into the standard set.
// Pass nil when no extra attributes are needed.
func (c *Client) CollectAllNodes(ctx context.Context, pageSize int, extraAttrs map[string][]string) ([]SearchResultRow, error) {
	if pageSize <= 0 {
		pageSize = 1000
	}

	attrs := NodeSearchAttributesWithExtra(extraAttrs)

	// First page — discover total count.
	first, err := c.PartialSearch(ctx, "node", "*:*", pageSize, 0, attrs)
	if err != nil {
		return nil, fmt.Errorf("chefapi: first page of node search: %w", err)
	}

	allRows := make([]SearchResultRow, 0, first.Total)
	allRows = append(allRows, first.Rows...)

	// Fetch remaining pages.
	for start := pageSize; start < first.Total; start += pageSize {
		page, err := c.PartialSearch(ctx, "node", "*:*", pageSize, start, attrs)
		if err != nil {
			return nil, fmt.Errorf("chefapi: node search page at start=%d: %w", start, err)
		}
		allRows = append(allRows, page.Rows...)
	}

	return allRows, nil
}

// PageResult holds the result of fetching a single page, used by the
// concurrent collector.
type PageResult struct {
	Start int
	Rows  []SearchResultRow
	Err   error
}

// CollectAllNodesConcurrent performs a paginated partial search to collect
// all nodes, fetching pages concurrently up to the given concurrency limit.
// It first fetches page 0 to discover the total count, then dispatches
// remaining pages in parallel.
//
// The optional extraAttrs parameter supplies additional partial-search keys
// (e.g. CMDB ownership attributes) that are merged into the standard set.
// Pass nil when no extra attributes are needed.
func (c *Client) CollectAllNodesConcurrent(ctx context.Context, pageSize, concurrency int, extraAttrs map[string][]string) ([]SearchResultRow, error) {
	if pageSize <= 0 {
		pageSize = 1000
	}
	if concurrency <= 0 {
		concurrency = 1
	}

	attrs := NodeSearchAttributesWithExtra(extraAttrs)

	// First page — discover total.
	first, err := c.PartialSearch(ctx, "node", "*:*", pageSize, 0, attrs)
	if err != nil {
		return nil, fmt.Errorf("chefapi: first page of node search: %w", err)
	}

	totalPages := 1
	if first.Total > pageSize {
		totalPages = int(math.Ceil(float64(first.Total) / float64(pageSize)))
	}

	if totalPages <= 1 {
		return first.Rows, nil
	}

	// Channel for page offsets to fetch.
	work := make(chan int, totalPages-1)
	for start := pageSize; start < first.Total; start += pageSize {
		work <- start
	}
	close(work)

	// Channel for results.
	results := make(chan PageResult, totalPages-1)

	// Launch workers.
	workerCount := concurrency
	if workerCount > totalPages-1 {
		workerCount = totalPages - 1
	}
	for i := 0; i < workerCount; i++ {
		go func() {
			for start := range work {
				if ctx.Err() != nil {
					results <- PageResult{Start: start, Err: ctx.Err()}
					continue
				}
				page, err := c.PartialSearch(ctx, "node", "*:*", pageSize, start, attrs)
				if err != nil {
					results <- PageResult{Start: start, Err: err}
				} else {
					results <- PageResult{Start: start, Rows: page.Rows}
				}
			}
		}()
	}

	// Collect results into a map keyed by start offset for ordered assembly.
	pageMap := make(map[int][]SearchResultRow, totalPages)
	pageMap[0] = first.Rows

	var firstErr error
	for i := 0; i < totalPages-1; i++ {
		pr := <-results
		if pr.Err != nil && firstErr == nil {
			firstErr = fmt.Errorf("chefapi: node search page at start=%d: %w", pr.Start, pr.Err)
		}
		if pr.Err == nil {
			pageMap[pr.Start] = pr.Rows
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}

	// Assemble rows in page order.
	allRows := make([]SearchResultRow, 0, first.Total)
	for start := 0; start < first.Total; start += pageSize {
		allRows = append(allRows, pageMap[start]...)
	}

	return allRows, nil
}

// ---------------------------------------------------------------------------
// Role and Cookbook endpoints
// ---------------------------------------------------------------------------

// GetRoles returns all role names for the organisation.
func (c *Client) GetRoles(ctx context.Context) ([]string, error) {
	respBody, err := c.doRequest(ctx, "GET", "/roles", nil)
	if err != nil {
		return nil, err
	}

	// Response is {"role_name": "url", ...}
	var rolesMap map[string]string
	if err := json.Unmarshal(respBody, &rolesMap); err != nil {
		return nil, fmt.Errorf("chefapi: unmarshalling roles list: %w", err)
	}

	names := make([]string, 0, len(rolesMap))
	for name := range rolesMap {
		names = append(names, name)
	}
	return names, nil
}

// RoleDetail holds the parsed detail of a Chef role.
type RoleDetail struct {
	Name        string              `json:"name"`
	RunList     []string            `json:"run_list"`
	EnvRunLists map[string][]string `json:"env_run_lists"`
	Description string              `json:"description"`
}

// GetRole returns the detail for a single role.
func (c *Client) GetRole(ctx context.Context, name string) (*RoleDetail, error) {
	path := "/roles/" + url.PathEscape(name)
	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var role RoleDetail
	if err := json.Unmarshal(respBody, &role); err != nil {
		return nil, fmt.Errorf("chefapi: unmarshalling role detail: %w", err)
	}
	return &role, nil
}

// CookbookListEntry represents a single cookbook in the cookbook list response.
type CookbookListEntry struct {
	URL      string                 `json:"url"`
	Versions []CookbookVersionEntry `json:"versions"`
}

// CookbookVersionEntry represents a version within a cookbook list entry.
type CookbookVersionEntry struct {
	URL     string `json:"url"`
	Version string `json:"version"`
}

// ---------------------------------------------------------------------------
// Cookbook version manifest types
// ---------------------------------------------------------------------------

// CookbookVersionManifest represents the parsed response from
// GET /organizations/ORG/cookbooks/NAME/VERSION. It contains the cookbook
// metadata and file manifest with download URLs for each file. The file
// download URLs point to the Chef server's bookshelf (file storage) and
// are typically pre-signed — they do not require Chef API authentication.
type CookbookVersionManifest struct {
	CookbookName string            `json:"cookbook_name"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Frozen       bool              `json:"frozen?"`
	Metadata     json.RawMessage   `json:"metadata"`
	Recipes      []CookbookFileRef `json:"recipes"`
	Definitions  []CookbookFileRef `json:"definitions"`
	Libraries    []CookbookFileRef `json:"libraries"`
	Attributes   []CookbookFileRef `json:"attributes"`
	Files        []CookbookFileRef `json:"files"`
	Templates    []CookbookFileRef `json:"templates"`
	Resources    []CookbookFileRef `json:"resources"`
	Providers    []CookbookFileRef `json:"providers"`
	RootFiles    []CookbookFileRef `json:"root_files"`
}

// CookbookMetadata holds the fields we extract from the Chef API metadata
// blob returned in a cookbook version manifest. The metadata object in the
// Chef API response contains many fields; we persist only the subset that
// is useful for migration assessment.
type CookbookMetadata struct {
	Maintainer      string            `json:"maintainer"`
	Description     string            `json:"description"`
	LongDescription string            `json:"long_description"`
	License         string            `json:"license"`
	Platforms       map[string]string `json:"platforms"`
	Dependencies    map[string]string `json:"dependencies"`
}

// ParseMetadata deserialises the raw JSON metadata blob from the cookbook
// version manifest into a CookbookMetadata struct. Only the fields defined
// on CookbookMetadata are extracted; all other metadata fields are silently
// ignored. Returns a zero-value CookbookMetadata (not an error) if the
// Metadata field is nil or empty, so callers can always use the result.
func (m *CookbookVersionManifest) ParseMetadata() (CookbookMetadata, error) {
	var meta CookbookMetadata
	if len(m.Metadata) == 0 {
		return meta, nil
	}
	if err := json.Unmarshal(m.Metadata, &meta); err != nil {
		return meta, fmt.Errorf("chefapi: parsing cookbook metadata for %s/%s: %w", m.CookbookName, m.Version, err)
	}
	return meta, nil
}

// CookbookFileRef describes a single file in a cookbook version manifest.
// The URL field contains a pre-signed download URL (typically to the Chef
// server's bookshelf/S3 storage). The Path field is the relative path
// within the cookbook (e.g. "recipes/default.rb").
type CookbookFileRef struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Checksum    string `json:"checksum"`
	Specificity string `json:"specificity"`
	URL         string `json:"url"`
}

// AllFiles returns a flat slice of all file references across all manifest
// categories (recipes, attributes, files, templates, etc.). This is useful
// for iterating over every file that needs to be downloaded.
func (m *CookbookVersionManifest) AllFiles() []CookbookFileRef {
	total := len(m.Recipes) + len(m.Definitions) + len(m.Libraries) +
		len(m.Attributes) + len(m.Files) + len(m.Templates) +
		len(m.Resources) + len(m.Providers) + len(m.RootFiles)

	all := make([]CookbookFileRef, 0, total)
	all = append(all, m.Recipes...)
	all = append(all, m.Definitions...)
	all = append(all, m.Libraries...)
	all = append(all, m.Attributes...)
	all = append(all, m.Files...)
	all = append(all, m.Templates...)
	all = append(all, m.Resources...)
	all = append(all, m.Providers...)
	all = append(all, m.RootFiles...)
	return all
}

// GetCookbooks returns all cookbooks and their versions for the organisation.
func (c *Client) GetCookbooks(ctx context.Context) (map[string]CookbookListEntry, error) {
	respBody, err := c.doRequest(ctx, "GET", "/cookbooks?num_versions=all", nil)
	if err != nil {
		return nil, err
	}

	var result map[string]CookbookListEntry
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("chefapi: unmarshalling cookbooks list: %w", err)
	}
	return result, nil
}

// GetCookbookVersion returns the raw JSON detail for a specific cookbook version.
func (c *Client) GetCookbookVersion(ctx context.Context, name, version string) (json.RawMessage, error) {
	path := fmt.Sprintf("/cookbooks/%s/%s", url.PathEscape(name), url.PathEscape(version))
	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(respBody), nil
}

// GetCookbookVersionManifest fetches and parses the cookbook version manifest
// from the Chef server. The manifest contains metadata and file references
// with pre-signed download URLs for each file in the cookbook.
func (c *Client) GetCookbookVersionManifest(ctx context.Context, name, version string) (*CookbookVersionManifest, error) {
	raw, err := c.GetCookbookVersion(ctx, name, version)
	if err != nil {
		return nil, err
	}

	var manifest CookbookVersionManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("chefapi: unmarshalling cookbook version manifest for %s/%s: %w", name, version, err)
	}
	return &manifest, nil
}

// DownloadFileContent downloads the content of a single cookbook file from
// its bookshelf URL. The bookshelf URLs are pre-signed and do not require
// Chef API authentication — a plain HTTP GET is used. The returned byte
// slice contains the raw file content.
//
// The checksum parameter, if non-empty, is validated against a SHA-256 hash
// of the downloaded content. A mismatch returns an error.
func (c *Client) DownloadFileContent(ctx context.Context, fileURL, checksum string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("chefapi: creating file download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chefapi: downloading file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Method:     "GET",
			Path:       fileURL,
			Body:       string(body),
		}
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("chefapi: reading file content: %w", err)
	}

	// Validate checksum if provided. Chef Server bookshelf checksums are
	// hex-encoded MD5 hashes (32 hex chars). We detect the algorithm from
	// the checksum length: 32 hex chars = MD5, 64 hex chars = SHA-256.
	if checksum != "" {
		var actual string
		switch len(checksum) {
		case 32: // MD5
			hash := md5.Sum(data)
			actual = fmt.Sprintf("%x", hash)
		default: // SHA-256 (or any other length — fall back to SHA-256)
			hash := sha256.Sum256(data)
			actual = fmt.Sprintf("%x", hash)
		}
		if actual != checksum {
			return nil, fmt.Errorf("chefapi: checksum mismatch for %s: expected %s, got %s", fileURL, checksum, actual)
		}
	}

	return data, nil
}

// ---------------------------------------------------------------------------
// Retry helper
// ---------------------------------------------------------------------------

// RetryConfig controls retry behaviour for transient failures.
type RetryConfig struct {
	MaxAttempts int           // Total attempts including the first (default: 3)
	InitialWait time.Duration // Wait before the first retry (default: 1s)
	MaxWait     time.Duration // Maximum wait between retries (default: 30s)
	Multiplier  float64       // Backoff multiplier (default: 2.0)
}

// DefaultRetryConfig returns a reasonable default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		InitialWait: 1 * time.Second,
		MaxWait:     30 * time.Second,
		Multiplier:  2.0,
	}
}

// DoWithRetry executes fn with exponential backoff retry for retryable errors
// (429 and 5xx). Non-retryable errors are returned immediately.
func DoWithRetry[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialWait <= 0 {
		cfg.InitialWait = 1 * time.Second
	}
	if cfg.MaxWait <= 0 {
		cfg.MaxWait = 30 * time.Second
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = 2.0
	}

	var lastErr error
	wait := cfg.InitialWait

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		// Check if retryable.
		apiErr, ok := err.(*APIError)
		if !ok || !apiErr.IsRetryable() {
			return result, err
		}

		lastErr = err

		// Don't sleep after the last attempt.
		if attempt < cfg.MaxAttempts-1 {
			// Use a timer that respects context cancellation.
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				var zero T
				return zero, ctx.Err()
			case <-timer.C:
			}

			// Increase wait with backoff.
			wait = time.Duration(float64(wait) * cfg.Multiplier)
			if wait > cfg.MaxWait {
				wait = cfg.MaxWait
			}
		}
	}

	var zero T
	return zero, fmt.Errorf("chefapi: max retries (%d) exceeded: %w", cfg.MaxAttempts, lastErr)
}

// ---------------------------------------------------------------------------
// Node data extraction helpers
// ---------------------------------------------------------------------------

// NodeData provides typed access to the fields returned by a node partial
// search. All fields are optional — getters return zero values if the field
// is missing or has an unexpected type.
type NodeData struct {
	Raw map[string]interface{}
}

// NewNodeData wraps a search result row's Data map.
func NewNodeData(data map[string]interface{}) NodeData {
	return NodeData{Raw: data}
}

func (n NodeData) getString(key string) string {
	v, _ := n.Raw[key].(string)
	return v
}

func (n NodeData) getFloat(key string) float64 {
	v, _ := n.Raw[key].(float64)
	return v
}

// Name returns the node name.
func (n NodeData) Name() string { return n.getString("name") }

// ChefEnvironment returns the node's Chef environment.
func (n NodeData) ChefEnvironment() string { return n.getString("chef_environment") }

// ChefVersion returns the Chef client version string.
func (n NodeData) ChefVersion() string { return n.getString("chef_version") }

// Platform returns the node's platform (e.g. "ubuntu").
func (n NodeData) Platform() string { return n.getString("platform") }

// PlatformVersion returns the platform version (e.g. "22.04").
func (n NodeData) PlatformVersion() string { return n.getString("platform_version") }

// PlatformFamily returns the platform family (e.g. "debian").
func (n NodeData) PlatformFamily() string { return n.getString("platform_family") }

// PolicyName returns the Policyfile policy name, or "" for classic nodes.
func (n NodeData) PolicyName() string { return n.getString("policy_name") }

// PolicyGroup returns the Policyfile policy group, or "" for classic nodes.
func (n NodeData) PolicyGroup() string { return n.getString("policy_group") }

// IsPolicyfileNode returns true if both policy_name and policy_group are set.
func (n NodeData) IsPolicyfileNode() bool {
	return n.PolicyName() != "" && n.PolicyGroup() != ""
}

// OhaiTime returns the ohai_time as a float64 Unix timestamp.
func (n NodeData) OhaiTime() float64 { return n.getFloat("ohai_time") }

// OhaiTimeAsTime converts ohai_time to a time.Time. Returns the zero time
// if ohai_time is not set.
func (n NodeData) OhaiTimeAsTime() time.Time {
	t := n.OhaiTime()
	if t == 0 {
		return time.Time{}
	}
	sec := int64(t)
	nsec := int64((t - float64(sec)) * 1e9)
	return time.Unix(sec, nsec)
}

// IsStale returns true if the node's ohai_time is older than the given
// threshold duration.
func (n NodeData) IsStale(threshold time.Duration) bool {
	t := n.OhaiTimeAsTime()
	if t.IsZero() {
		return true // No ohai_time means we can't tell — treat as stale.
	}
	return time.Since(t) > threshold
}

// Cookbooks returns the cookbooks map from the node data. Each key is a
// cookbook name and the value is a map that typically contains "version".
func (n NodeData) Cookbooks() map[string]map[string]interface{} {
	raw, ok := n.Raw["cookbooks"].(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string]map[string]interface{}, len(raw))
	for name, v := range raw {
		if m, ok := v.(map[string]interface{}); ok {
			result[name] = m
		}
	}
	return result
}

// CookbookVersions returns a simplified map of cookbook name → version string.
func (n NodeData) CookbookVersions() map[string]string {
	cbs := n.Cookbooks()
	if cbs == nil {
		return nil
	}
	result := make(map[string]string, len(cbs))
	for name, meta := range cbs {
		if v, ok := meta["version"].(string); ok {
			result[name] = v
		}
	}
	return result
}

// RunList returns the node's run_list as a string slice.
func (n NodeData) RunList() []string {
	raw, ok := n.Raw["run_list"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// Roles returns the node's expanded roles as a string slice.
func (n NodeData) Roles() []string {
	raw, ok := n.Raw["roles"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// Filesystem returns the raw filesystem data. The structure varies by
// platform and Ohai version; callers should handle it defensively.
func (n NodeData) Filesystem() map[string]interface{} {
	m, _ := n.Raw["filesystem"].(map[string]interface{})
	return m
}

// FreeDiskMB attempts to extract the available disk space in MB for the
// root filesystem ("/"). Returns -1 if the data is unavailable.
func (n NodeData) FreeDiskMB() int64 {
	fs := n.Filesystem()
	if fs == nil {
		return -1
	}

	// Ohai reports filesystem data in different structures depending on
	// the plugin version. Try the "by_mountpoint" key first, then fall
	// back to looking for "/" directly.
	var root map[string]interface{}

	if byMount, ok := fs["by_mountpoint"].(map[string]interface{}); ok {
		if r, ok := byMount["/"].(map[string]interface{}); ok {
			root = r
		}
	}
	if root == nil {
		if r, ok := fs["/"].(map[string]interface{}); ok {
			root = r
		}
	}
	if root == nil {
		return -1
	}

	// Look for kb_available.
	switch v := root["kb_available"].(type) {
	case float64:
		return int64(v) / 1024
	case string:
		if kb, err := strconv.ParseInt(v, 10, 64); err == nil {
			return kb / 1024
		}
	}

	return -1
}
