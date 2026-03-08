// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fixedClock returns a clock function that always returns the given time.
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// refTime is a fixed reference time used throughout the tests.
var refTime = time.Date(2025, 1, 20, 12, 0, 0, 0, time.UTC)

// newTestLogger creates a logger with a MemoryWriter and fixed clock.
func newTestLogger(level Severity) (*Logger, *MemoryWriter) {
	mw := NewMemoryWriter()
	l := New(Options{
		Level:   level,
		Writers: []Writer{mw},
		Clock:   fixedClock(refTime),
	})
	return l, mw
}

// ---------------------------------------------------------------------------
// Severity tests
// ---------------------------------------------------------------------------

func TestSeverity_String(t *testing.T) {
	tests := []struct {
		sev  Severity
		want string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
		{Severity(99), "UNKNOWN(99)"},
		{Severity(-1), "UNKNOWN(-1)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.sev.String(); got != tt.want {
				t.Errorf("Severity(%d).String() = %q, want %q", int(tt.sev), got, tt.want)
			}
		})
	}
}

func TestSeverity_Ordering(t *testing.T) {
	if DEBUG >= INFO || INFO >= WARN || WARN >= ERROR {
		t.Fatal("severity ordering is incorrect: expected DEBUG < INFO < WARN < ERROR")
	}
}

func TestParseSeverity_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  Severity
	}{
		{"DEBUG", DEBUG},
		{"debug", DEBUG},
		{"Debug", DEBUG},
		{"  DEBUG  ", DEBUG},
		{"INFO", INFO},
		{"info", INFO},
		{"WARN", WARN},
		{"warn", WARN},
		{"ERROR", ERROR},
		{"error", ERROR},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSeverity(tt.input)
			if err != nil {
				t.Fatalf("ParseSeverity(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseSeverity(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSeverity_Invalid(t *testing.T) {
	tests := []string{"", "FATAL", "TRACE", "warning", "123", "  "}
	for _, input := range tests {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			got, err := ParseSeverity(input)
			if err == nil {
				t.Fatalf("ParseSeverity(%q) returned no error, expected error", input)
			}
			if got != INFO {
				t.Errorf("ParseSeverity(%q) = %v, want INFO as default", input, got)
			}
			if !strings.Contains(err.Error(), "unknown severity") {
				t.Errorf("error message %q should contain 'unknown severity'", err.Error())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Scope tests
// ---------------------------------------------------------------------------

func TestIsValidScope(t *testing.T) {
	validScopes := []Scope{
		ScopeCollectionRun,
		ScopeGitOperation,
		ScopeTestKitchenRun,
		ScopeCookstyleScan,
		ScopeNotificationDispatch,
		ScopeExportJob,
		ScopeTLS,
		ScopeReadinessEvaluation,
		ScopeStartup,
		ScopeSecrets,
		ScopeRemediation,
	}
	for _, s := range validScopes {
		t.Run(string(s), func(t *testing.T) {
			if !IsValidScope(s) {
				t.Errorf("IsValidScope(%q) = false, want true", s)
			}
		})
	}
}

func TestIsValidScope_Invalid(t *testing.T) {
	invalid := []Scope{"", "unknown", "collection", "STARTUP", "Collection_Run"}
	for _, s := range invalid {
		t.Run(string(s), func(t *testing.T) {
			if IsValidScope(s) {
				t.Errorf("IsValidScope(%q) = true, want false", s)
			}
		})
	}
}

func TestScope_StringValues(t *testing.T) {
	// Verify the string values match the spec exactly.
	tests := []struct {
		scope Scope
		want  string
	}{
		{ScopeCollectionRun, "collection_run"},
		{ScopeGitOperation, "git_operation"},
		{ScopeTestKitchenRun, "test_kitchen_run"},
		{ScopeCookstyleScan, "cookstyle_scan"},
		{ScopeNotificationDispatch, "notification_dispatch"},
		{ScopeExportJob, "export_job"},
		{ScopeTLS, "tls"},
		{ScopeReadinessEvaluation, "readiness_evaluation"},
		{ScopeStartup, "startup"},
		{ScopeSecrets, "secrets"},
		{ScopeRemediation, "remediation"},
	}
	for _, tt := range tests {
		if string(tt.scope) != tt.want {
			t.Errorf("scope constant %q has value %q, want %q", tt.want, string(tt.scope), tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Entry tests
// ---------------------------------------------------------------------------

func TestEntry_MarshalJSON(t *testing.T) {
	e := Entry{
		Timestamp:    refTime,
		Severity:     WARN,
		Scope:        ScopeCollectionRun,
		Message:      "something happened",
		Organisation: "prod-org",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Severity should be a string, not a number.
	sev, ok := decoded["severity"].(string)
	if !ok {
		t.Fatalf("severity field is not a string: %T", decoded["severity"])
	}
	if sev != "WARN" {
		t.Errorf("severity = %q, want %q", sev, "WARN")
	}

	if decoded["scope"] != "collection_run" {
		t.Errorf("scope = %v, want %q", decoded["scope"], "collection_run")
	}
	if decoded["message"] != "something happened" {
		t.Errorf("message = %v, want %q", decoded["message"], "something happened")
	}
	if decoded["organisation"] != "prod-org" {
		t.Errorf("organisation = %v, want %q", decoded["organisation"], "prod-org")
	}
}

func TestEntry_MarshalJSON_OmitsEmptyFields(t *testing.T) {
	e := Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeStartup,
		Message:   "started",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	for _, field := range []string{
		"organisation", "cookbook_name", "cookbook_version", "commit_sha",
		"chef_client_version", "process_output", "collection_run_id",
		"notification_channel", "export_job_id", "tls_domain",
	} {
		if _, exists := decoded[field]; exists {
			t.Errorf("empty field %q should be omitted from JSON, but was present", field)
		}
	}
}

func TestEntry_MarshalJSON_AllSeverities(t *testing.T) {
	for _, sev := range []Severity{DEBUG, INFO, WARN, ERROR} {
		t.Run(sev.String(), func(t *testing.T) {
			e := Entry{Timestamp: refTime, Severity: sev, Scope: ScopeStartup, Message: "test"}
			data, err := json.Marshal(e)
			if err != nil {
				t.Fatalf("MarshalJSON failed: %v", err)
			}
			if !strings.Contains(string(data), fmt.Sprintf(`"severity":"%s"`, sev.String())) {
				t.Errorf("JSON %s does not contain severity string %q", string(data), sev.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Option tests
// ---------------------------------------------------------------------------

func TestOptions_WithOrganisation(t *testing.T) {
	e := Entry{}
	WithOrganisation("my-org")(&e)
	if e.Organisation != "my-org" {
		t.Errorf("Organisation = %q, want %q", e.Organisation, "my-org")
	}
}

func TestOptions_WithCookbook(t *testing.T) {
	e := Entry{}
	WithCookbook("apache2", "5.2.1")(&e)
	if e.CookbookName != "apache2" {
		t.Errorf("CookbookName = %q, want %q", e.CookbookName, "apache2")
	}
	if e.CookbookVersion != "5.2.1" {
		t.Errorf("CookbookVersion = %q, want %q", e.CookbookVersion, "5.2.1")
	}
}

func TestOptions_WithCookbook_NameOnly(t *testing.T) {
	e := Entry{}
	WithCookbook("nginx", "")(&e)
	if e.CookbookName != "nginx" {
		t.Errorf("CookbookName = %q, want %q", e.CookbookName, "nginx")
	}
	if e.CookbookVersion != "" {
		t.Errorf("CookbookVersion = %q, want empty", e.CookbookVersion)
	}
}

func TestOptions_WithCommitSHA(t *testing.T) {
	e := Entry{}
	WithCommitSHA("abc123def")(&e)
	if e.CommitSHA != "abc123def" {
		t.Errorf("CommitSHA = %q, want %q", e.CommitSHA, "abc123def")
	}
}

func TestOptions_WithChefClientVersion(t *testing.T) {
	e := Entry{}
	WithChefClientVersion("18.4.12")(&e)
	if e.ChefClientVersion != "18.4.12" {
		t.Errorf("ChefClientVersion = %q, want %q", e.ChefClientVersion, "18.4.12")
	}
}

func TestOptions_WithProcessOutput(t *testing.T) {
	e := Entry{}
	output := "line 1\nline 2\nline 3"
	WithProcessOutput(output)(&e)
	if e.ProcessOutput != output {
		t.Errorf("ProcessOutput = %q, want %q", e.ProcessOutput, output)
	}
}

func TestOptions_WithCollectionRunID(t *testing.T) {
	e := Entry{}
	WithCollectionRunID("run-abc-123")(&e)
	if e.CollectionRunID != "run-abc-123" {
		t.Errorf("CollectionRunID = %q, want %q", e.CollectionRunID, "run-abc-123")
	}
}

func TestOptions_WithNotificationChannel(t *testing.T) {
	e := Entry{}
	WithNotificationChannel("slack-ops")(&e)
	if e.NotificationChannel != "slack-ops" {
		t.Errorf("NotificationChannel = %q, want %q", e.NotificationChannel, "slack-ops")
	}
}

func TestOptions_WithExportJobID(t *testing.T) {
	e := Entry{}
	WithExportJobID("export-456")(&e)
	if e.ExportJobID != "export-456" {
		t.Errorf("ExportJobID = %q, want %q", e.ExportJobID, "export-456")
	}
}

func TestOptions_WithTLSDomain(t *testing.T) {
	e := Entry{}
	WithTLSDomain("example.com")(&e)
	if e.TLSDomain != "example.com" {
		t.Errorf("TLSDomain = %q, want %q", e.TLSDomain, "example.com")
	}
}

func TestOptions_MultipleOptionsApplied(t *testing.T) {
	e := Entry{}
	opts := []Option{
		WithOrganisation("org1"),
		WithCookbook("cb", "1.0"),
		WithCommitSHA("sha1"),
		WithChefClientVersion("19.0"),
		WithCollectionRunID("run-1"),
		WithNotificationChannel("email-team"),
		WithExportJobID("exp-1"),
		WithTLSDomain("test.example.com"),
		WithProcessOutput("output data"),
	}
	for _, opt := range opts {
		opt(&e)
	}

	if e.Organisation != "org1" {
		t.Errorf("Organisation = %q, want %q", e.Organisation, "org1")
	}
	if e.CookbookName != "cb" || e.CookbookVersion != "1.0" {
		t.Errorf("Cookbook = %q@%q, want cb@1.0", e.CookbookName, e.CookbookVersion)
	}
	if e.CommitSHA != "sha1" {
		t.Errorf("CommitSHA = %q, want sha1", e.CommitSHA)
	}
	if e.ChefClientVersion != "19.0" {
		t.Errorf("ChefClientVersion = %q, want 19.0", e.ChefClientVersion)
	}
	if e.CollectionRunID != "run-1" {
		t.Errorf("CollectionRunID = %q, want run-1", e.CollectionRunID)
	}
	if e.NotificationChannel != "email-team" {
		t.Errorf("NotificationChannel = %q, want email-team", e.NotificationChannel)
	}
	if e.ExportJobID != "exp-1" {
		t.Errorf("ExportJobID = %q, want exp-1", e.ExportJobID)
	}
	if e.TLSDomain != "test.example.com" {
		t.Errorf("TLSDomain = %q, want test.example.com", e.TLSDomain)
	}
	if e.ProcessOutput != "output data" {
		t.Errorf("ProcessOutput = %q, want output data", e.ProcessOutput)
	}
}

func TestOptions_LaterOptionOverridesEarlier(t *testing.T) {
	e := Entry{}
	WithOrganisation("first")(&e)
	WithOrganisation("second")(&e)
	if e.Organisation != "second" {
		t.Errorf("Organisation = %q, want %q (later option should win)", e.Organisation, "second")
	}
}

// ---------------------------------------------------------------------------
// StdoutWriter tests
// ---------------------------------------------------------------------------

func TestStdoutWriter_HumanFormat_BasicMessage(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStdoutWriter(WithOutput(&buf))

	entry := Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeStartup,
		Message:   "application started",
	}

	if err := sw.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry failed: %v", err)
	}

	line := buf.String()
	if !strings.Contains(line, "2025-01-20T12:00:00Z") {
		t.Errorf("line %q does not contain expected timestamp", line)
	}
	if !strings.Contains(line, "INFO") {
		t.Errorf("line %q does not contain INFO", line)
	}
	if !strings.Contains(line, "[startup]") {
		t.Errorf("line %q does not contain [startup]", line)
	}
	if !strings.Contains(line, "application started") {
		t.Errorf("line %q does not contain message", line)
	}
	if !strings.HasSuffix(line, "\n") {
		t.Error("line does not end with newline")
	}
}

func TestStdoutWriter_HumanFormat_WithMetadata(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStdoutWriter(WithOutput(&buf))

	entry := Entry{
		Timestamp:       refTime,
		Severity:        WARN,
		Scope:           ScopeCookstyleScan,
		Message:         "deprecation found",
		Organisation:    "prod",
		CookbookName:    "apache2",
		CookbookVersion: "5.2.1",
		CommitSHA:       "abc123",
		CollectionRunID: "run-1",
	}

	if err := sw.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry failed: %v", err)
	}

	line := buf.String()
	if !strings.Contains(line, "org=prod") {
		t.Errorf("line %q does not contain org=prod", line)
	}
	if !strings.Contains(line, "cookbook=apache2@5.2.1") {
		t.Errorf("line %q does not contain cookbook=apache2@5.2.1", line)
	}
	if !strings.Contains(line, "commit=abc123") {
		t.Errorf("line %q does not contain commit=abc123", line)
	}
	if !strings.Contains(line, "run_id=run-1") {
		t.Errorf("line %q does not contain run_id=run-1", line)
	}
}

func TestStdoutWriter_HumanFormat_CookbookWithoutVersion(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStdoutWriter(WithOutput(&buf))

	entry := Entry{
		Timestamp:    refTime,
		Severity:     INFO,
		Scope:        ScopeGitOperation,
		Message:      "cloned",
		CookbookName: "nginx",
	}

	if err := sw.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry failed: %v", err)
	}

	line := buf.String()
	if !strings.Contains(line, "cookbook=nginx") {
		t.Errorf("line %q does not contain cookbook=nginx", line)
	}
	if strings.Contains(line, "cookbook=nginx@") {
		t.Errorf("line %q should not contain version separator when version is empty", line)
	}
}

func TestStdoutWriter_HumanFormat_AllMetadataFields(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStdoutWriter(WithOutput(&buf))

	entry := Entry{
		Timestamp:           refTime,
		Severity:            ERROR,
		Scope:               ScopeNotificationDispatch,
		Message:             "delivery failed",
		Organisation:        "org1",
		CookbookName:        "cb",
		CookbookVersion:     "1.0",
		CommitSHA:           "sha",
		ChefClientVersion:   "19.0",
		CollectionRunID:     "run",
		NotificationChannel: "slack",
		ExportJobID:         "exp",
		TLSDomain:           "example.com",
	}

	if err := sw.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry failed: %v", err)
	}

	line := buf.String()
	for _, want := range []string{
		"org=org1", "cookbook=cb@1.0", "commit=sha", "chef_version=19.0",
		"run_id=run", "channel=slack", "export_job=exp", "domain=example.com",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q does not contain %q", line, want)
		}
	}
}

func TestStdoutWriter_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStdoutWriter(WithOutput(&buf), WithJSON(true))

	entry := Entry{
		Timestamp:    refTime,
		Severity:     ERROR,
		Scope:        ScopeCollectionRun,
		Message:      "auth failure",
		Organisation: "dev-org",
	}

	if err := sw.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry failed: %v", err)
	}

	line := buf.String()
	if !strings.HasSuffix(line, "\n") {
		t.Error("JSON output does not end with newline")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, line)
	}

	if decoded["severity"] != "ERROR" {
		t.Errorf("severity = %v, want ERROR", decoded["severity"])
	}
	if decoded["organisation"] != "dev-org" {
		t.Errorf("organisation = %v, want dev-org", decoded["organisation"])
	}
}

func TestStdoutWriter_DefaultOutput(t *testing.T) {
	// Verify the default output is os.Stdout (just check it doesn't panic).
	sw := NewStdoutWriter()
	// We can't easily capture os.Stdout in a test, but we can verify the
	// writer was created without error.
	if sw == nil {
		t.Fatal("NewStdoutWriter() returned nil")
	}
}

func TestStdoutWriter_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStdoutWriter(WithOutput(&buf))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			entry := Entry{
				Timestamp: refTime,
				Severity:  INFO,
				Scope:     ScopeStartup,
				Message:   fmt.Sprintf("message-%d", n),
			}
			_ = sw.WriteEntry(entry)
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 50 {
		t.Errorf("got %d lines, want 50", len(lines))
	}
}

// ---------------------------------------------------------------------------
// formatMeta tests
// ---------------------------------------------------------------------------

func TestFormatMeta_Empty(t *testing.T) {
	e := Entry{}
	if got := formatMeta(e); got != "" {
		t.Errorf("formatMeta(empty) = %q, want empty string", got)
	}
}

func TestFormatMeta_SingleField(t *testing.T) {
	e := Entry{Organisation: "myorg"}
	got := formatMeta(e)
	if got != "org=myorg" {
		t.Errorf("formatMeta = %q, want %q", got, "org=myorg")
	}
}

func TestFormatMeta_MultipleFields(t *testing.T) {
	e := Entry{
		Organisation: "org1",
		CookbookName: "cb",
		TLSDomain:    "example.com",
	}
	got := formatMeta(e)
	// Should be space-separated.
	if !strings.Contains(got, "org=org1") {
		t.Errorf("missing org=org1 in %q", got)
	}
	if !strings.Contains(got, "cookbook=cb") {
		t.Errorf("missing cookbook=cb in %q", got)
	}
	if !strings.Contains(got, "domain=example.com") {
		t.Errorf("missing domain=example.com in %q", got)
	}
}

func TestFormatMeta_ProcessOutputNotIncluded(t *testing.T) {
	e := Entry{ProcessOutput: "some long output"}
	got := formatMeta(e)
	if got != "" {
		t.Errorf("formatMeta should not include process_output, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Logger tests
// ---------------------------------------------------------------------------

func TestNew_DefaultWriter(t *testing.T) {
	// When no writers are provided, a default StdoutWriter is used.
	l := New(Options{Level: INFO})
	if l == nil {
		t.Fatal("New returned nil")
	}
	if len(l.writers) != 1 {
		t.Errorf("expected 1 default writer, got %d", len(l.writers))
	}
}

func TestNew_DefaultClock(t *testing.T) {
	l := New(Options{Level: INFO, Writers: []Writer{NewMemoryWriter()}})
	now := l.clock()
	// Should be roughly time.Now().
	if time.Since(now) > 2*time.Second {
		t.Error("default clock returned a time too far from now")
	}
}

func TestLogger_Level(t *testing.T) {
	l, _ := newTestLogger(WARN)
	if l.Level() != WARN {
		t.Errorf("Level() = %v, want %v", l.Level(), WARN)
	}
}

func TestLogger_Info(t *testing.T) {
	l, mw := newTestLogger(INFO)

	err := l.Info(ScopeStartup, "server started")
	if err != nil {
		t.Fatalf("Info() returned error: %v", err)
	}

	entries := mw.Entries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	e := entries[0]
	if e.Severity != INFO {
		t.Errorf("Severity = %v, want INFO", e.Severity)
	}
	if e.Scope != ScopeStartup {
		t.Errorf("Scope = %v, want %v", e.Scope, ScopeStartup)
	}
	if e.Message != "server started" {
		t.Errorf("Message = %q, want %q", e.Message, "server started")
	}
	if !e.Timestamp.Equal(refTime) {
		t.Errorf("Timestamp = %v, want %v", e.Timestamp, refTime)
	}
}

func TestLogger_Debug(t *testing.T) {
	l, mw := newTestLogger(DEBUG)
	_ = l.Debug(ScopeCollectionRun, "debug message")
	if mw.Len() != 1 {
		t.Fatalf("got %d entries, want 1", mw.Len())
	}
	if mw.Entries()[0].Severity != DEBUG {
		t.Error("expected DEBUG severity")
	}
}

func TestLogger_Warn(t *testing.T) {
	l, mw := newTestLogger(DEBUG)
	_ = l.Warn(ScopeTLS, "certificate expiring")
	if mw.Len() != 1 {
		t.Fatalf("got %d entries, want 1", mw.Len())
	}
	if mw.Entries()[0].Severity != WARN {
		t.Error("expected WARN severity")
	}
}

func TestLogger_Error(t *testing.T) {
	l, mw := newTestLogger(DEBUG)
	_ = l.Error(ScopeCollectionRun, "connection refused")
	if mw.Len() != 1 {
		t.Fatalf("got %d entries, want 1", mw.Len())
	}
	if mw.Entries()[0].Severity != ERROR {
		t.Error("expected ERROR severity")
	}
}

func TestLogger_Logf(t *testing.T) {
	l, mw := newTestLogger(DEBUG)
	_ = l.Logf(INFO, ScopeStartup, "applied %d migration(s)", 3)
	entries := mw.Entries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Message != "applied 3 migration(s)" {
		t.Errorf("Message = %q, want %q", entries[0].Message, "applied 3 migration(s)")
	}
}

func TestLogger_WithOptions(t *testing.T) {
	l, mw := newTestLogger(INFO)

	_ = l.Info(ScopeCollectionRun, "collected nodes",
		WithOrganisation("prod"),
		WithCollectionRunID("run-123"),
	)

	entries := mw.Entries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	e := entries[0]
	if e.Organisation != "prod" {
		t.Errorf("Organisation = %q, want %q", e.Organisation, "prod")
	}
	if e.CollectionRunID != "run-123" {
		t.Errorf("CollectionRunID = %q, want %q", e.CollectionRunID, "run-123")
	}
}

func TestLogger_TimestampIsUTC(t *testing.T) {
	// Ensure the logger always stores UTC timestamps even when the clock
	// returns a non-UTC time.
	est := time.FixedZone("EST", -5*60*60)
	localTime := time.Date(2025, 1, 20, 7, 0, 0, 0, est) // 12:00 UTC

	mw := NewMemoryWriter()
	l := New(Options{
		Level:   INFO,
		Writers: []Writer{mw},
		Clock:   fixedClock(localTime),
	})

	_ = l.Info(ScopeStartup, "test")

	e := mw.Entries()[0]
	if e.Timestamp.Location() != time.UTC {
		t.Errorf("Timestamp location = %v, want UTC", e.Timestamp.Location())
	}
	if !e.Timestamp.Equal(localTime.UTC()) {
		t.Errorf("Timestamp = %v, want %v", e.Timestamp, localTime.UTC())
	}
}

// ---------------------------------------------------------------------------
// Level filtering tests
// ---------------------------------------------------------------------------

func TestLogger_LevelFiltering_DebugLevel(t *testing.T) {
	l, mw := newTestLogger(DEBUG)
	_ = l.Debug(ScopeStartup, "d")
	_ = l.Info(ScopeStartup, "i")
	_ = l.Warn(ScopeStartup, "w")
	_ = l.Error(ScopeStartup, "e")
	if mw.Len() != 4 {
		t.Errorf("DEBUG level should pass all 4, got %d", mw.Len())
	}
}

func TestLogger_LevelFiltering_InfoLevel(t *testing.T) {
	l, mw := newTestLogger(INFO)
	_ = l.Debug(ScopeStartup, "d")
	_ = l.Info(ScopeStartup, "i")
	_ = l.Warn(ScopeStartup, "w")
	_ = l.Error(ScopeStartup, "e")
	if mw.Len() != 3 {
		t.Errorf("INFO level should pass 3 (INFO/WARN/ERROR), got %d", mw.Len())
	}
}

func TestLogger_LevelFiltering_WarnLevel(t *testing.T) {
	l, mw := newTestLogger(WARN)
	_ = l.Debug(ScopeStartup, "d")
	_ = l.Info(ScopeStartup, "i")
	_ = l.Warn(ScopeStartup, "w")
	_ = l.Error(ScopeStartup, "e")
	if mw.Len() != 2 {
		t.Errorf("WARN level should pass 2 (WARN/ERROR), got %d", mw.Len())
	}
}

func TestLogger_LevelFiltering_ErrorLevel(t *testing.T) {
	l, mw := newTestLogger(ERROR)
	_ = l.Debug(ScopeStartup, "d")
	_ = l.Info(ScopeStartup, "i")
	_ = l.Warn(ScopeStartup, "w")
	_ = l.Error(ScopeStartup, "e")
	if mw.Len() != 1 {
		t.Errorf("ERROR level should pass 1 (ERROR only), got %d", mw.Len())
	}
}

func TestLogger_LevelFiltering_BelowMinimumReturnsNil(t *testing.T) {
	l, _ := newTestLogger(ERROR)
	err := l.Debug(ScopeStartup, "should be discarded")
	if err != nil {
		t.Errorf("logging below minimum level should return nil, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Multi-writer tests
// ---------------------------------------------------------------------------

func TestLogger_MultipleWriters(t *testing.T) {
	mw1 := NewMemoryWriter()
	mw2 := NewMemoryWriter()
	l := New(Options{
		Level:   INFO,
		Writers: []Writer{mw1, mw2},
		Clock:   fixedClock(refTime),
	})

	_ = l.Info(ScopeStartup, "test")

	if mw1.Len() != 1 {
		t.Errorf("writer 1 got %d entries, want 1", mw1.Len())
	}
	if mw2.Len() != 1 {
		t.Errorf("writer 2 got %d entries, want 1", mw2.Len())
	}
}

func TestLogger_WriterError_SingleWriter(t *testing.T) {
	ew := &ErrorWriter{Err: errors.New("write failed")}
	l := New(Options{
		Level:   INFO,
		Writers: []Writer{ew},
		Clock:   fixedClock(refTime),
	})

	err := l.Info(ScopeStartup, "test")
	if err == nil {
		t.Fatal("expected error from failing writer")
	}
	if !strings.Contains(err.Error(), "write failed") {
		t.Errorf("error %q should contain 'write failed'", err.Error())
	}
}

func TestLogger_WriterError_DoesNotBlockOtherWriters(t *testing.T) {
	ew := &ErrorWriter{Err: errors.New("write failed")}
	mw := NewMemoryWriter()
	l := New(Options{
		Level:   INFO,
		Writers: []Writer{ew, mw},
		Clock:   fixedClock(refTime),
	})

	err := l.Info(ScopeStartup, "test")
	if err == nil {
		t.Fatal("expected error when one writer fails")
	}
	// The memory writer should still have received the entry.
	if mw.Len() != 1 {
		t.Errorf("memory writer got %d entries, want 1 (error in one writer should not block others)", mw.Len())
	}
}

func TestLogger_MultipleWriterErrors(t *testing.T) {
	ew1 := &ErrorWriter{Err: errors.New("fail-1")}
	ew2 := &ErrorWriter{Err: errors.New("fail-2")}
	l := New(Options{
		Level:   INFO,
		Writers: []Writer{ew1, ew2},
		Clock:   fixedClock(refTime),
	})

	err := l.Info(ScopeStartup, "test")
	if err == nil {
		t.Fatal("expected combined error")
	}
	if !strings.Contains(err.Error(), "multiple writer errors") {
		t.Errorf("error %q should mention 'multiple writer errors'", err.Error())
	}
	if !strings.Contains(err.Error(), "fail-1") || !strings.Contains(err.Error(), "fail-2") {
		t.Errorf("error %q should contain both failure messages", err.Error())
	}
}

func TestLogger_ConcurrentLogging(t *testing.T) {
	mw := NewMemoryWriter()
	l := New(Options{
		Level:   DEBUG,
		Writers: []Writer{mw},
		Clock:   fixedClock(refTime),
	})

	var wg sync.WaitGroup
	count := 100
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = l.Info(ScopeStartup, fmt.Sprintf("msg-%d", n))
		}(i)
	}
	wg.Wait()

	if mw.Len() != count {
		t.Errorf("got %d entries, want %d", mw.Len(), count)
	}
}

// ---------------------------------------------------------------------------
// ScopedLogger tests
// ---------------------------------------------------------------------------

func TestScopedLogger_FixesScope(t *testing.T) {
	l, mw := newTestLogger(DEBUG)
	sl := l.WithScope(ScopeCollectionRun)

	_ = sl.Info("started")
	_ = sl.Warn("slow")
	_ = sl.Error("failed")
	_ = sl.Debug("detail")

	entries := mw.Entries()
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}
	for i, e := range entries {
		if e.Scope != ScopeCollectionRun {
			t.Errorf("entry %d: Scope = %v, want %v", i, e.Scope, ScopeCollectionRun)
		}
	}

	// Verify severity of each.
	if entries[0].Severity != INFO {
		t.Error("entry 0 should be INFO")
	}
	if entries[1].Severity != WARN {
		t.Error("entry 1 should be WARN")
	}
	if entries[2].Severity != ERROR {
		t.Error("entry 2 should be ERROR")
	}
	if entries[3].Severity != DEBUG {
		t.Error("entry 3 should be DEBUG")
	}
}

func TestScopedLogger_Scope(t *testing.T) {
	l, _ := newTestLogger(INFO)
	sl := l.WithScope(ScopeTLS)
	if sl.Scope() != ScopeTLS {
		t.Errorf("Scope() = %v, want %v", sl.Scope(), ScopeTLS)
	}
}

func TestScopedLogger_BaseOptions(t *testing.T) {
	l, mw := newTestLogger(INFO)
	sl := l.WithScope(ScopeCollectionRun,
		WithOrganisation("base-org"),
		WithCollectionRunID("run-base"),
	)

	_ = sl.Info("test")

	e := mw.Entries()[0]
	if e.Organisation != "base-org" {
		t.Errorf("Organisation = %q, want %q", e.Organisation, "base-org")
	}
	if e.CollectionRunID != "run-base" {
		t.Errorf("CollectionRunID = %q, want %q", e.CollectionRunID, "run-base")
	}
}

func TestScopedLogger_PerCallOptionsOverrideBase(t *testing.T) {
	l, mw := newTestLogger(INFO)
	sl := l.WithScope(ScopeCollectionRun,
		WithOrganisation("base-org"),
	)

	// Per-call option overrides the base option.
	_ = sl.Info("test", WithOrganisation("override-org"))

	e := mw.Entries()[0]
	if e.Organisation != "override-org" {
		t.Errorf("Organisation = %q, want %q (per-call should override base)", e.Organisation, "override-org")
	}
}

func TestScopedLogger_PerCallOptionsAddToBase(t *testing.T) {
	l, mw := newTestLogger(INFO)
	sl := l.WithScope(ScopeCollectionRun,
		WithOrganisation("org1"),
	)

	_ = sl.Info("test", WithCookbook("nginx", "2.0"))

	e := mw.Entries()[0]
	if e.Organisation != "org1" {
		t.Errorf("Organisation = %q, want %q", e.Organisation, "org1")
	}
	if e.CookbookName != "nginx" {
		t.Errorf("CookbookName = %q, want %q", e.CookbookName, "nginx")
	}
}

func TestScopedLogger_RespectsLogLevel(t *testing.T) {
	l, mw := newTestLogger(WARN)
	sl := l.WithScope(ScopeStartup)

	_ = sl.Debug("d")
	_ = sl.Info("i")
	_ = sl.Warn("w")
	_ = sl.Error("e")

	if mw.Len() != 2 {
		t.Errorf("WARN-level scoped logger should pass 2 entries, got %d", mw.Len())
	}
}

func TestScopedLogger_NoBaseOptions(t *testing.T) {
	l, mw := newTestLogger(INFO)
	sl := l.WithScope(ScopeExportJob)

	_ = sl.Info("exported", WithExportJobID("job-1"))

	e := mw.Entries()[0]
	if e.ExportJobID != "job-1" {
		t.Errorf("ExportJobID = %q, want %q", e.ExportJobID, "job-1")
	}
	if e.Scope != ScopeExportJob {
		t.Errorf("Scope = %v, want %v", e.Scope, ScopeExportJob)
	}
}

// ---------------------------------------------------------------------------
// MemoryWriter tests
// ---------------------------------------------------------------------------

func TestMemoryWriter_EntriesReturnsCopy(t *testing.T) {
	mw := NewMemoryWriter()
	_ = mw.WriteEntry(Entry{Message: "a"})
	_ = mw.WriteEntry(Entry{Message: "b"})

	entries1 := mw.Entries()
	entries2 := mw.Entries()

	if len(entries1) != 2 || len(entries2) != 2 {
		t.Fatalf("expected 2 entries each, got %d and %d", len(entries1), len(entries2))
	}

	// Mutating the first slice should not affect the second.
	entries1[0].Message = "mutated"
	if entries2[0].Message == "mutated" {
		t.Error("Entries() should return a copy, but mutation was visible")
	}
}

func TestMemoryWriter_Len(t *testing.T) {
	mw := NewMemoryWriter()
	if mw.Len() != 0 {
		t.Errorf("Len() = %d, want 0", mw.Len())
	}
	_ = mw.WriteEntry(Entry{Message: "a"})
	if mw.Len() != 1 {
		t.Errorf("Len() = %d, want 1", mw.Len())
	}
}

func TestMemoryWriter_Reset(t *testing.T) {
	mw := NewMemoryWriter()
	_ = mw.WriteEntry(Entry{Message: "a"})
	_ = mw.WriteEntry(Entry{Message: "b"})
	mw.Reset()
	if mw.Len() != 0 {
		t.Errorf("Len() after Reset = %d, want 0", mw.Len())
	}
}

func TestMemoryWriter_ConcurrentAccess(t *testing.T) {
	mw := NewMemoryWriter()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = mw.WriteEntry(Entry{Message: fmt.Sprintf("msg-%d", n)})
		}(i)
	}
	wg.Wait()

	if mw.Len() != 50 {
		t.Errorf("Len() = %d, want 50", mw.Len())
	}
}

// ---------------------------------------------------------------------------
// ErrorWriter tests
// ---------------------------------------------------------------------------

func TestErrorWriter_AlwaysFails(t *testing.T) {
	want := errors.New("boom")
	ew := &ErrorWriter{Err: want}
	err := ew.WriteEntry(Entry{})
	if !errors.Is(err, want) {
		t.Errorf("WriteEntry error = %v, want %v", err, want)
	}
}

// ---------------------------------------------------------------------------
// DBWriter tests
// ---------------------------------------------------------------------------

func TestDBWriter_PersistsEntryToInserter(t *testing.T) {
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)

	entry := Entry{
		Timestamp:           refTime,
		Severity:            WARN,
		Scope:               ScopeCookstyleScan,
		Message:             "deprecation found",
		Organisation:        "prod",
		CookbookName:        "apache2",
		CookbookVersion:     "5.2.1",
		CommitSHA:           "abc123",
		ChefClientVersion:   "19.0",
		ProcessOutput:       "line 1\nline 2",
		CollectionRunID:     "run-1",
		NotificationChannel: "slack",
		ExportJobID:         "exp-1",
		TLSDomain:           "example.com",
	}

	err := dw.WriteEntry(entry)
	if err != nil {
		t.Fatalf("WriteEntry returned error: %v", err)
	}

	if rec.Len() != 1 {
		t.Fatalf("got %d recorded entries, want 1", rec.Len())
	}

	p := rec.Entries()[0]
	if p.Severity != "WARN" {
		t.Errorf("Severity = %q, want WARN", p.Severity)
	}
	if p.Scope != "cookstyle_scan" {
		t.Errorf("Scope = %q, want cookstyle_scan", p.Scope)
	}
	if p.Message != "deprecation found" {
		t.Errorf("Message = %q, want 'deprecation found'", p.Message)
	}
	if p.Organisation != "prod" {
		t.Errorf("Organisation = %q, want prod", p.Organisation)
	}
	if p.CookbookName != "apache2" {
		t.Errorf("CookbookName = %q, want apache2", p.CookbookName)
	}
	if p.CookbookVersion != "5.2.1" {
		t.Errorf("CookbookVersion = %q, want 5.2.1", p.CookbookVersion)
	}
	if p.CommitSHA != "abc123" {
		t.Errorf("CommitSHA = %q, want abc123", p.CommitSHA)
	}
	if p.ChefClientVersion != "19.0" {
		t.Errorf("ChefClientVersion = %q, want 19.0", p.ChefClientVersion)
	}
	if p.ProcessOutput != "line 1\nline 2" {
		t.Errorf("ProcessOutput = %q, want 'line 1\\nline 2'", p.ProcessOutput)
	}
	if p.CollectionRunID != "run-1" {
		t.Errorf("CollectionRunID = %q, want run-1", p.CollectionRunID)
	}
	if p.NotificationChannel != "slack" {
		t.Errorf("NotificationChannel = %q, want slack", p.NotificationChannel)
	}
	if p.ExportJobID != "exp-1" {
		t.Errorf("ExportJobID = %q, want exp-1", p.ExportJobID)
	}
	if p.TLSDomain != "example.com" {
		t.Errorf("TLSDomain = %q, want example.com", p.TLSDomain)
	}
	if !p.Timestamp.Equal(refTime) {
		t.Errorf("Timestamp = %v, want %v", p.Timestamp, refTime)
	}
}

func TestDBWriter_InsertFailureSwallowed(t *testing.T) {
	fi := &FailingDBInserter{Err: errors.New("db down")}
	dw := NewDBWriter(fi)

	err := dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeStartup,
		Message:   "test",
	})
	// Error should be swallowed — WriteEntry returns nil.
	if err != nil {
		t.Errorf("WriteEntry should swallow DB errors, got %v", err)
	}
}

func TestDBWriter_InsertFailureCallsOnError(t *testing.T) {
	dbErr := errors.New("db down")
	fi := &FailingDBInserter{Err: dbErr}

	var capturedEntry Entry
	var capturedErr error
	onError := func(entry Entry, err error) {
		capturedEntry = entry
		capturedErr = err
	}

	dw := NewDBWriter(fi, WithOnError(onError))

	entry := Entry{
		Timestamp: refTime,
		Severity:  ERROR,
		Scope:     ScopeStartup,
		Message:   "important",
	}
	_ = dw.WriteEntry(entry)

	if capturedErr == nil {
		t.Fatal("OnError callback was not invoked")
	}
	if !errors.Is(capturedErr, dbErr) {
		t.Errorf("OnError error = %v, want %v", capturedErr, dbErr)
	}
	if capturedEntry.Message != "important" {
		t.Errorf("OnError entry message = %q, want %q", capturedEntry.Message, "important")
	}
}

func TestDBWriter_InsertFailureNoOnError(t *testing.T) {
	fi := &FailingDBInserter{Err: errors.New("db down")}
	dw := NewDBWriter(fi) // no OnError callback

	// Should not panic.
	err := dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeStartup,
		Message:   "test",
	})
	if err != nil {
		t.Errorf("WriteEntry should swallow errors even without OnError, got %v", err)
	}
}

func TestDBWriter_WithContext(t *testing.T) {
	rec := NewRecordingDBInserter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dw := NewDBWriter(rec, WithContext(ctx))

	_ = dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeStartup,
		Message:   "test",
	})
	if rec.Len() != 1 {
		t.Errorf("got %d entries, want 1", rec.Len())
	}
}

func TestDBWriter_SetOnError(t *testing.T) {
	dbErr := errors.New("fail")
	fi := &FailingDBInserter{Err: dbErr}
	dw := NewDBWriter(fi)

	var called bool
	dw.SetOnError(func(_ Entry, _ error) {
		called = true
	})

	_ = dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeStartup,
		Message:   "test",
	})
	if !called {
		t.Error("SetOnError callback was not invoked")
	}
}

func TestDBWriter_ConcurrentWrites(t *testing.T) {
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)

	var wg sync.WaitGroup
	count := 100
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = dw.WriteEntry(Entry{
				Timestamp: refTime,
				Severity:  INFO,
				Scope:     ScopeStartup,
				Message:   fmt.Sprintf("msg-%d", n),
			})
		}(i)
	}
	wg.Wait()

	if rec.Len() != count {
		t.Errorf("got %d entries, want %d", rec.Len(), count)
	}
}

func TestDBWriter_NilInserterPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil inserter")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T, want string", r)
		}
		if !strings.Contains(msg, "nil inserter") {
			t.Errorf("panic message %q should mention nil inserter", msg)
		}
	}()
	NewDBWriter(nil)
}

func TestDBWriter_SeverityConvertedToString(t *testing.T) {
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)

	for _, sev := range []Severity{DEBUG, INFO, WARN, ERROR} {
		rec.Reset()
		_ = dw.WriteEntry(Entry{
			Timestamp: refTime,
			Severity:  sev,
			Scope:     ScopeStartup,
			Message:   "test",
		})
		if rec.Len() != 1 {
			t.Fatalf("expected 1 entry for severity %v", sev)
		}
		if rec.Entries()[0].Severity != sev.String() {
			t.Errorf("recorded severity = %q, want %q", rec.Entries()[0].Severity, sev.String())
		}
	}
}

func TestDBWriter_ScopeConvertedToString(t *testing.T) {
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)

	_ = dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeCollectionRun,
		Message:   "test",
	})

	if rec.Entries()[0].Scope != "collection_run" {
		t.Errorf("recorded scope = %q, want %q", rec.Entries()[0].Scope, "collection_run")
	}
}

// ---------------------------------------------------------------------------
// RecordingDBInserter tests
// ---------------------------------------------------------------------------

func TestRecordingDBInserter_SequentialIDs(t *testing.T) {
	rec := NewRecordingDBInserter()
	id1, err := rec.InsertLogEntry(context.Background(), LogEntryParams{Message: "a"})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := rec.InsertLogEntry(context.Background(), LogEntryParams{Message: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 {
		t.Errorf("IDs should be unique, got %q and %q", id1, id2)
	}
	if id1 != "log-1" || id2 != "log-2" {
		t.Errorf("IDs = %q, %q; want log-1, log-2", id1, id2)
	}
}

func TestRecordingDBInserter_Reset(t *testing.T) {
	rec := NewRecordingDBInserter()
	_, _ = rec.InsertLogEntry(context.Background(), LogEntryParams{Message: "a"})
	rec.Reset()
	if rec.Len() != 0 {
		t.Errorf("Len after reset = %d, want 0", rec.Len())
	}
	// IDs should restart.
	id, _ := rec.InsertLogEntry(context.Background(), LogEntryParams{Message: "b"})
	if id != "log-1" {
		t.Errorf("ID after reset = %q, want log-1", id)
	}
}

func TestRecordingDBInserter_ConcurrentAccess(t *testing.T) {
	rec := NewRecordingDBInserter()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = rec.InsertLogEntry(context.Background(), LogEntryParams{Message: fmt.Sprintf("msg-%d", n)})
		}(i)
	}
	wg.Wait()
	if rec.Len() != 50 {
		t.Errorf("Len = %d, want 50", rec.Len())
	}
}

// ---------------------------------------------------------------------------
// FailingDBInserter tests
// ---------------------------------------------------------------------------

func TestFailingDBInserter_CustomError(t *testing.T) {
	want := errors.New("custom error")
	fi := &FailingDBInserter{Err: want}
	_, err := fi.InsertLogEntry(context.Background(), LogEntryParams{})
	if !errors.Is(err, want) {
		t.Errorf("error = %v, want %v", err, want)
	}
}

func TestFailingDBInserter_DefaultError(t *testing.T) {
	fi := &FailingDBInserter{} // nil Err
	_, err := fi.InsertLogEntry(context.Background(), LogEntryParams{})
	if err == nil {
		t.Fatal("expected error from FailingDBInserter with nil Err")
	}
	if !strings.Contains(err.Error(), "simulated DB insert failure") {
		t.Errorf("error %q should contain default message", err.Error())
	}
}

// ---------------------------------------------------------------------------
// DatastoreAdapter tests
// ---------------------------------------------------------------------------

func TestDatastoreAdapter_DelegatesToFunction(t *testing.T) {
	var captured LogEntryParams
	adapter := NewDatastoreAdapter(func(_ context.Context, p LogEntryParams) (string, error) {
		captured = p
		return "test-id-42", nil
	})

	params := LogEntryParams{
		Severity: "INFO",
		Scope:    "startup",
		Message:  "hello",
	}
	id, err := adapter.InsertLogEntry(context.Background(), params)
	if err != nil {
		t.Fatalf("InsertLogEntry returned error: %v", err)
	}
	if id != "test-id-42" {
		t.Errorf("ID = %q, want %q", id, "test-id-42")
	}
	if captured.Message != "hello" {
		t.Errorf("captured message = %q, want %q", captured.Message, "hello")
	}
}

func TestDatastoreAdapter_PropagatesError(t *testing.T) {
	want := errors.New("insert failed")
	adapter := NewDatastoreAdapter(func(_ context.Context, _ LogEntryParams) (string, error) {
		return "", want
	})

	_, err := adapter.InsertLogEntry(context.Background(), LogEntryParams{})
	if !errors.Is(err, want) {
		t.Errorf("error = %v, want %v", err, want)
	}
}

func TestDatastoreAdapter_NilFunctionPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil function")
		}
	}()
	NewDatastoreAdapter(nil)
}

// ---------------------------------------------------------------------------
// Integration: Logger + DBWriter + StdoutWriter
// ---------------------------------------------------------------------------

func TestLogger_DBWriterAndStdoutWriter(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStdoutWriter(WithOutput(&buf))
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)

	l := New(Options{
		Level:   INFO,
		Writers: []Writer{sw, dw},
		Clock:   fixedClock(refTime),
	})

	_ = l.Info(ScopeCollectionRun, "collected 150 nodes",
		WithOrganisation("production"),
		WithCollectionRunID("run-abc"),
	)

	// StdoutWriter captured the entry.
	stdout := buf.String()
	if !strings.Contains(stdout, "collected 150 nodes") {
		t.Errorf("stdout %q should contain message", stdout)
	}
	if !strings.Contains(stdout, "org=production") {
		t.Errorf("stdout %q should contain org=production", stdout)
	}

	// DBWriter captured the entry.
	if rec.Len() != 1 {
		t.Fatalf("DB inserter got %d entries, want 1", rec.Len())
	}
	p := rec.Entries()[0]
	if p.Organisation != "production" {
		t.Errorf("DB entry organisation = %q, want production", p.Organisation)
	}
	if p.CollectionRunID != "run-abc" {
		t.Errorf("DB entry collection_run_id = %q, want run-abc", p.CollectionRunID)
	}
}

func TestLogger_DBWriterFailsButStdoutContinues(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStdoutWriter(WithOutput(&buf))
	fi := &FailingDBInserter{Err: errors.New("db down")}
	dw := NewDBWriter(fi)

	l := New(Options{
		Level:   INFO,
		Writers: []Writer{sw, dw},
		Clock:   fixedClock(refTime),
	})

	err := l.Info(ScopeStartup, "test message")
	// DBWriter swallows errors, so the logger should return nil.
	if err != nil {
		t.Errorf("expected nil error when DBWriter fails silently, got %v", err)
	}

	// StdoutWriter should still have the entry.
	if !strings.Contains(buf.String(), "test message") {
		t.Error("stdout should still contain the message even when DB fails")
	}
}

// ---------------------------------------------------------------------------
// Integration: ScopedLogger with DBWriter
// ---------------------------------------------------------------------------

func TestScopedLogger_WithDBWriter(t *testing.T) {
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)

	l := New(Options{
		Level:   INFO,
		Writers: []Writer{dw},
		Clock:   fixedClock(refTime),
	})

	sl := l.WithScope(ScopeTestKitchenRun,
		WithOrganisation("staging"),
		WithCookbook("haproxy", "12.0.0"),
		WithChefClientVersion("19.4.0"),
	)

	_ = sl.Info("convergence passed")
	_ = sl.Warn("slow convergence", WithProcessOutput("took 300s"))

	entries := rec.Entries()
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	// First entry: base options only.
	e0 := entries[0]
	if e0.Scope != "test_kitchen_run" {
		t.Errorf("e0 scope = %q, want test_kitchen_run", e0.Scope)
	}
	if e0.Organisation != "staging" {
		t.Errorf("e0 org = %q, want staging", e0.Organisation)
	}
	if e0.CookbookName != "haproxy" || e0.CookbookVersion != "12.0.0" {
		t.Errorf("e0 cookbook = %q@%q, want haproxy@12.0.0", e0.CookbookName, e0.CookbookVersion)
	}
	if e0.ChefClientVersion != "19.4.0" {
		t.Errorf("e0 chef_version = %q, want 19.4.0", e0.ChefClientVersion)
	}
	if e0.ProcessOutput != "" {
		t.Errorf("e0 should not have process output, got %q", e0.ProcessOutput)
	}

	// Second entry: base options + per-call ProcessOutput.
	e1 := entries[1]
	if e1.Organisation != "staging" {
		t.Errorf("e1 org = %q, want staging (from base)", e1.Organisation)
	}
	if e1.ProcessOutput != "took 300s" {
		t.Errorf("e1 process_output = %q, want 'took 300s'", e1.ProcessOutput)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestLogger_EmptyMessage(t *testing.T) {
	l, mw := newTestLogger(INFO)
	_ = l.Info(ScopeStartup, "")
	if mw.Len() != 1 {
		t.Fatal("empty message should still be logged")
	}
	if mw.Entries()[0].Message != "" {
		t.Error("message should be empty string")
	}
}

func TestLogger_NoOptions(t *testing.T) {
	l, mw := newTestLogger(INFO)
	_ = l.Info(ScopeStartup, "no options")
	e := mw.Entries()[0]
	if e.Organisation != "" || e.CookbookName != "" || e.CollectionRunID != "" {
		t.Error("fields should be empty when no options are provided")
	}
}

func TestScopedLogger_MergeOptsWithNilBase(t *testing.T) {
	l, mw := newTestLogger(INFO)
	sl := l.WithScope(ScopeStartup) // no base opts

	_ = sl.Info("test", WithOrganisation("org"))
	e := mw.Entries()[0]
	if e.Organisation != "org" {
		t.Errorf("Organisation = %q, want org", e.Organisation)
	}
}

func TestScopedLogger_MergeOptsWithNilExtra(t *testing.T) {
	l, mw := newTestLogger(INFO)
	sl := l.WithScope(ScopeStartup, WithOrganisation("base"))

	_ = sl.Info("test") // no extra opts
	e := mw.Entries()[0]
	if e.Organisation != "base" {
		t.Errorf("Organisation = %q, want base", e.Organisation)
	}
}

// ---------------------------------------------------------------------------
// Datastore log_entries validation tests
// ---------------------------------------------------------------------------

func TestValidateLogEntryParams(t *testing.T) {
	// This tests the validation helper in the datastore package via the
	// types we mirror here. Since we can't directly call
	// datastore.validateLogEntryParams, we test that the DBWriter correctly
	// maps severity values to strings that the datastore would accept.
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)

	// All four severity levels should produce valid severity strings.
	for _, sev := range []Severity{DEBUG, INFO, WARN, ERROR} {
		_ = dw.WriteEntry(Entry{
			Timestamp: refTime,
			Severity:  sev,
			Scope:     ScopeStartup,
			Message:   "test",
		})
	}

	entries := rec.Entries()
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}

	expectedSeverities := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	for i, p := range entries {
		if p.Severity != expectedSeverities[i] {
			t.Errorf("entry %d severity = %q, want %q", i, p.Severity, expectedSeverities[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Large process output test
// ---------------------------------------------------------------------------

func TestDBWriter_LargeProcessOutput(t *testing.T) {
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)

	// Simulate a large Test Kitchen output.
	largeOutput := strings.Repeat("line of output\n", 1000) // ~15KB
	_ = dw.WriteEntry(Entry{
		Timestamp:     refTime,
		Severity:      INFO,
		Scope:         ScopeTestKitchenRun,
		Message:       "convergence complete",
		ProcessOutput: largeOutput,
	})

	if rec.Len() != 1 {
		t.Fatal("expected 1 entry")
	}
	if rec.Entries()[0].ProcessOutput != largeOutput {
		t.Error("large process output was not preserved intact")
	}
}

// ---------------------------------------------------------------------------
// Realistic workflow test
// ---------------------------------------------------------------------------

func TestRealisticCollectionRunWorkflow(t *testing.T) {
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)
	var buf bytes.Buffer
	sw := NewStdoutWriter(WithOutput(&buf))

	l := New(Options{
		Level:   INFO,
		Writers: []Writer{sw, dw},
		Clock:   fixedClock(refTime),
	})

	// Simulate a collection run for "production" org.
	sl := l.WithScope(ScopeCollectionRun,
		WithOrganisation("production"),
		WithCollectionRunID("run-2025-01-20"),
	)

	_ = sl.Info("collection run started")
	_ = sl.Info("fetching nodes from Chef server")
	_ = sl.Info("collected 450 nodes")
	_ = sl.Warn("3 nodes have stale ohai_time")
	_ = sl.Info("collection run completed")

	// Simulate a CookStyle scan.
	csl := l.WithScope(ScopeCookstyleScan,
		WithOrganisation("production"),
		WithCookbook("java", "8.6.0"),
	)
	_ = csl.Info("starting CookStyle scan", WithChefClientVersion("19.4.0"))
	_ = csl.Warn("5 deprecation warnings found",
		WithChefClientVersion("19.4.0"),
		WithProcessOutput("Chef/Deprecations/ResourceUsesOnlyResourceName: ..."),
	)

	// Simulate a TLS event.
	_ = l.Info(ScopeTLS, "TLS mode: off")

	entries := rec.Entries()
	if len(entries) != 8 {
		t.Fatalf("got %d entries, want 8", len(entries))
	}

	// All collection_run entries should have org and run_id.
	for i := 0; i < 5; i++ {
		if entries[i].Scope != "collection_run" {
			t.Errorf("entry %d scope = %q, want collection_run", i, entries[i].Scope)
		}
		if entries[i].Organisation != "production" {
			t.Errorf("entry %d org = %q, want production", i, entries[i].Organisation)
		}
		if entries[i].CollectionRunID != "run-2025-01-20" {
			t.Errorf("entry %d run_id = %q, want run-2025-01-20", i, entries[i].CollectionRunID)
		}
	}

	// CookStyle entries.
	if entries[5].Scope != "cookstyle_scan" {
		t.Errorf("entry 5 scope = %q, want cookstyle_scan", entries[5].Scope)
	}
	if entries[5].CookbookName != "java" {
		t.Errorf("entry 5 cookbook = %q, want java", entries[5].CookbookName)
	}
	if entries[6].ProcessOutput == "" {
		t.Error("entry 6 should have process output")
	}

	// TLS entry.
	if entries[7].Scope != "tls" {
		t.Errorf("entry 7 scope = %q, want tls", entries[7].Scope)
	}

	// Stdout should have all 8 lines.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 8 {
		t.Errorf("stdout has %d lines, want 8", len(lines))
	}
}
