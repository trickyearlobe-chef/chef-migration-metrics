package webapi

import (
	"encoding/json"
	"os"
	"testing"
)

// Temporary scaffolding: writes the served description to the path in
// CMM_DUMP_OPENAPI so it can be compared with a probe recording.
func TestDumpOpenAPI(t *testing.T) {
	path := os.Getenv("CMM_DUMP_OPENAPI")
	if path == "" {
		t.Skip("CMM_DUMP_OPENAPI not set")
	}
	doc := newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).openAPIDocument()
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
