// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package logging provides structured logging for Chef Migration Metrics.
//
// All components emit structured log entries that are written to stdout (for
// operators) and optionally persisted to the PostgreSQL datastore (for the
// web UI log viewer). Each entry carries a severity, scope, human-readable
// message, and optional contextual metadata (organisation, cookbook name,
// commit SHA, etc.).
//
// # Severity Levels
//
// Four severity levels are defined in ascending order: DEBUG, INFO, WARN,
// ERROR. A configurable minimum level controls which entries are emitted;
// entries below the minimum are silently discarded.
//
// # Scopes
//
// Every log entry belongs to exactly one scope that identifies the unit of
// work being performed. See the Scope constants for the full list.
//
// # Database Persistence
//
// When a [DBWriter] is attached to the logger, entries are inserted into the
// log_entries table. The [DBWriter] also provides a PurgeExpired method for
// retention-based cleanup.
//
// # Usage
//
//	logger := logging.New(logging.Options{Level: logging.INFO})
//	logger.Info(logging.ScopeCollectionRun, "collection started",
//	    logging.WithOrganisation("production"),
//	)
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Severity
// ---------------------------------------------------------------------------

// Severity represents a log severity level.
type Severity int

const (
	// DEBUG is detailed diagnostic information, typically only useful during
	// development.
	DEBUG Severity = iota
	// INFO is normal operational events (job started, job completed, etc.).
	INFO
	// WARN is unexpected but recoverable conditions.
	WARN
	// ERROR is failures that require attention.
	ERROR
)

// String returns the uppercase name of the severity level.
func (s Severity) String() string {
	switch s {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

// ParseSeverity converts a string to a Severity. The comparison is
// case-insensitive. Returns INFO and an error for unrecognised values.
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return DEBUG, nil
	case "INFO":
		return INFO, nil
	case "WARN":
		return WARN, nil
	case "ERROR":
		return ERROR, nil
	default:
		return INFO, fmt.Errorf("logging: unknown severity %q, defaulting to INFO", s)
	}
}

// ---------------------------------------------------------------------------
// Scope
// ---------------------------------------------------------------------------

// Scope identifies the unit of work that produced a log entry.
type Scope string

const (
	// ScopeCollectionRun is a periodic node data collection run for a single
	// Chef server organisation.
	ScopeCollectionRun Scope = "collection_run"

	// ScopeGitOperation is a clone or pull operation for a single cookbook
	// git repository.
	ScopeGitOperation Scope = "git_operation"

	// ScopeTestKitchenRun is a Test Kitchen execution for a single cookbook
	// against a single target Chef Client version.
	ScopeTestKitchenRun Scope = "test_kitchen_run"

	// ScopeCookstyleScan is a CookStyle scan for a single cookbook version
	// sourced from the Chef server.
	ScopeCookstyleScan Scope = "cookstyle_scan"

	// ScopeNotificationDispatch is a notification delivery attempt to a
	// configured channel (webhook or email).
	ScopeNotificationDispatch Scope = "notification_dispatch"

	// ScopeExportJob is a data export operation (ready nodes, blocked nodes,
	// or cookbook remediation report).
	ScopeExportJob Scope = "export_job"

	// ScopeTLS covers TLS certificate lifecycle events — mode selection,
	// certificate loading, reload, ACME issuance, renewal, expiry warnings,
	// and errors.
	ScopeTLS Scope = "tls"

	// ScopeReadinessEvaluation is used for node upgrade readiness evaluation
	// operations.
	ScopeReadinessEvaluation Scope = "readiness_evaluation"

	// ScopeStartup is used for application startup events that don't belong
	// to a specific functional scope (config loaded, DB connected, migrations
	// applied, etc.).
	ScopeStartup Scope = "startup"

	// ScopeSecrets is used for credential lifecycle events — creation,
	// rotation, deletion, decryption failures, and startup validation of
	// the master encryption key and stored credentials.
	ScopeSecrets Scope = "secrets"

	// ScopeRemediation is used for remediation guidance operations —
	// auto-correct preview generation, cop-to-documentation enrichment,
	// and cookbook complexity scoring.
	ScopeRemediation Scope = "remediation"

	// ScopeWebAPI is used for HTTP API lifecycle events — request routing,
	// WebSocket connections, and handler-level logging.
	ScopeWebAPI Scope = "webapi"
)

// validScopes is the set of recognised scope values. Used for validation.
var validScopes = map[Scope]bool{
	ScopeCollectionRun:        true,
	ScopeGitOperation:         true,
	ScopeTestKitchenRun:       true,
	ScopeCookstyleScan:        true,
	ScopeNotificationDispatch: true,
	ScopeExportJob:            true,
	ScopeTLS:                  true,
	ScopeReadinessEvaluation:  true,
	ScopeStartup:              true,
	ScopeSecrets:              true,
	ScopeRemediation:          true,
	ScopeWebAPI:               true,
}

// IsValidScope returns true if s is a recognised scope value.
func IsValidScope(s Scope) bool {
	return validScopes[s]
}

// ---------------------------------------------------------------------------
// Entry
// ---------------------------------------------------------------------------

// Entry is a single structured log entry. It is created by the Logger and
// passed to writers for output.
type Entry struct {
	// Timestamp is the time at which the event occurred (UTC).
	Timestamp time.Time `json:"timestamp"`

	// Severity is the log level of the entry.
	Severity Severity `json:"severity"`

	// Scope identifies the unit of work that produced this entry.
	Scope Scope `json:"scope"`

	// Message is a human-readable description of the event.
	Message string `json:"message"`

	// Organisation is the Chef server organisation name, if applicable.
	Organisation string `json:"organisation,omitempty"`

	// CookbookName is the cookbook name, if applicable.
	CookbookName string `json:"cookbook_name,omitempty"`

	// CookbookVersion is the cookbook version, if applicable.
	CookbookVersion string `json:"cookbook_version,omitempty"`

	// CommitSHA is the git commit SHA, if applicable.
	CommitSHA string `json:"commit_sha,omitempty"`

	// ChefClientVersion is the target Chef Client version, if applicable.
	ChefClientVersion string `json:"chef_client_version,omitempty"`

	// ProcessOutput is captured stdout/stderr from an external process.
	ProcessOutput string `json:"process_output,omitempty"`

	// CollectionRunID links this entry to a specific collection run.
	CollectionRunID string `json:"collection_run_id,omitempty"`

	// NotificationChannel is the name of the notification channel, if
	// applicable (notification_dispatch scope).
	NotificationChannel string `json:"notification_channel,omitempty"`

	// ExportJobID is the export job identifier, if applicable (export_job
	// scope).
	ExportJobID string `json:"export_job_id,omitempty"`

	// TLSDomain is the domain name associated with a TLS certificate event,
	// if applicable (tls scope).
	TLSDomain string `json:"tls_domain,omitempty"`
}

// MarshalJSON implements json.Marshaler for Entry. The severity field is
// serialised as a string.
func (e Entry) MarshalJSON() ([]byte, error) {
	type Alias Entry
	return json.Marshal(struct {
		Severity string `json:"severity"`
		Alias
	}{
		Severity: e.Severity.String(),
		Alias:    (Alias)(e),
	})
}

// ---------------------------------------------------------------------------
// Entry options (functional options for contextual metadata)
// ---------------------------------------------------------------------------

// Option is a functional option that attaches contextual metadata to an
// Entry.
type Option func(*Entry)

// WithOrganisation sets the organisation field on the log entry.
func WithOrganisation(org string) Option {
	return func(e *Entry) { e.Organisation = org }
}

// WithCookbook sets the cookbook_name and optionally cookbook_version fields.
func WithCookbook(name, version string) Option {
	return func(e *Entry) {
		e.CookbookName = name
		e.CookbookVersion = version
	}
}

// WithCommitSHA sets the commit_sha field on the log entry.
func WithCommitSHA(sha string) Option {
	return func(e *Entry) { e.CommitSHA = sha }
}

// WithChefClientVersion sets the chef_client_version field on the log entry.
func WithChefClientVersion(v string) Option {
	return func(e *Entry) { e.ChefClientVersion = v }
}

// WithProcessOutput sets the process_output field on the log entry.
func WithProcessOutput(output string) Option {
	return func(e *Entry) { e.ProcessOutput = output }
}

// WithCollectionRunID sets the collection_run_id field on the log entry.
func WithCollectionRunID(id string) Option {
	return func(e *Entry) { e.CollectionRunID = id }
}

// WithNotificationChannel sets the notification_channel field on the log
// entry.
func WithNotificationChannel(channel string) Option {
	return func(e *Entry) { e.NotificationChannel = channel }
}

// WithExportJobID sets the export_job_id field on the log entry.
func WithExportJobID(id string) Option {
	return func(e *Entry) { e.ExportJobID = id }
}

// WithTLSDomain sets the tls_domain field on the log entry.
func WithTLSDomain(domain string) Option {
	return func(e *Entry) { e.TLSDomain = domain }
}

// ---------------------------------------------------------------------------
// Writer interface
// ---------------------------------------------------------------------------

// Writer is the interface for log output destinations. The Logger fans out
// each entry to all attached writers.
type Writer interface {
	// WriteEntry writes a single log entry. Implementations must be safe
	// for concurrent use.
	WriteEntry(entry Entry) error
}

// ---------------------------------------------------------------------------
// StdoutWriter
// ---------------------------------------------------------------------------

// StdoutWriter writes log entries to an io.Writer (typically os.Stdout or
// os.Stderr) in a human-readable format. It is safe for concurrent use.
type StdoutWriter struct {
	mu     sync.Mutex
	output io.Writer
	json   bool
}

// StdoutWriterOption configures a StdoutWriter.
type StdoutWriterOption func(*StdoutWriter)

// WithOutput sets the output destination for the StdoutWriter. The default
// is os.Stdout.
func WithOutput(w io.Writer) StdoutWriterOption {
	return func(sw *StdoutWriter) { sw.output = w }
}

// WithJSON enables JSON output format instead of the default human-readable
// format.
func WithJSON(enabled bool) StdoutWriterOption {
	return func(sw *StdoutWriter) { sw.json = enabled }
}

// NewStdoutWriter creates a new StdoutWriter with the given options.
func NewStdoutWriter(opts ...StdoutWriterOption) *StdoutWriter {
	sw := &StdoutWriter{
		output: os.Stdout,
	}
	for _, opt := range opts {
		opt(sw)
	}
	return sw
}

// WriteEntry writes a single log entry to the configured output. It formats
// the entry as either a human-readable line or a JSON object depending on
// configuration.
func (sw *StdoutWriter) WriteEntry(entry Entry) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.json {
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("logging: marshalling entry to JSON: %w", err)
		}
		_, err = fmt.Fprintf(sw.output, "%s\n", data)
		return err
	}

	// Human-readable format:
	// 2025-01-20T12:00:00Z INFO  [collection_run] collection started org=production
	ts := entry.Timestamp.UTC().Format(time.RFC3339)
	meta := formatMeta(entry)
	if meta != "" {
		_, err := fmt.Fprintf(sw.output, "%s %-5s [%s] %s %s\n",
			ts, entry.Severity, entry.Scope, entry.Message, meta)
		return err
	}
	_, err := fmt.Fprintf(sw.output, "%s %-5s [%s] %s\n",
		ts, entry.Severity, entry.Scope, entry.Message)
	return err
}

// formatMeta builds a key=value metadata string from an entry's non-empty
// contextual fields.
func formatMeta(e Entry) string {
	var parts []string
	if e.Organisation != "" {
		parts = append(parts, "org="+e.Organisation)
	}
	if e.CookbookName != "" {
		s := "cookbook=" + e.CookbookName
		if e.CookbookVersion != "" {
			s += "@" + e.CookbookVersion
		}
		parts = append(parts, s)
	}
	if e.CommitSHA != "" {
		parts = append(parts, "commit="+e.CommitSHA)
	}
	if e.ChefClientVersion != "" {
		parts = append(parts, "chef_version="+e.ChefClientVersion)
	}
	if e.CollectionRunID != "" {
		parts = append(parts, "run_id="+e.CollectionRunID)
	}
	if e.NotificationChannel != "" {
		parts = append(parts, "channel="+e.NotificationChannel)
	}
	if e.ExportJobID != "" {
		parts = append(parts, "export_job="+e.ExportJobID)
	}
	if e.TLSDomain != "" {
		parts = append(parts, "domain="+e.TLSDomain)
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Logger
// ---------------------------------------------------------------------------

// Options configures a Logger.
type Options struct {
	// Level is the minimum severity level. Entries below this level are
	// silently discarded. Defaults to INFO.
	Level Severity

	// Writers is the set of output destinations. If empty, a default
	// StdoutWriter is used.
	Writers []Writer

	// Clock is an optional function that returns the current time. If nil,
	// time.Now is used. This exists for testing.
	Clock func() time.Time
}

// Logger is the central structured logging type. It fans out each entry to
// all attached writers. It is safe for concurrent use.
type Logger struct {
	level   Severity
	writers []Writer
	clock   func() time.Time
}

// New creates a new Logger with the given options.
func New(opts Options) *Logger {
	writers := opts.Writers
	if len(writers) == 0 {
		writers = []Writer{NewStdoutWriter()}
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	return &Logger{
		level:   opts.Level,
		writers: writers,
		clock:   clock,
	}
}

// Level returns the minimum severity level of the logger.
func (l *Logger) Level() Severity {
	return l.level
}

// log creates an entry and fans it out to all writers. Errors from
// individual writers are collected but do not prevent other writers from
// receiving the entry.
func (l *Logger) log(severity Severity, scope Scope, msg string, opts ...Option) error {
	if severity < l.level {
		return nil
	}

	entry := Entry{
		Timestamp: l.clock().UTC(),
		Severity:  severity,
		Scope:     scope,
		Message:   msg,
	}
	for _, opt := range opts {
		opt(&entry)
	}

	var errs []error
	for _, w := range l.writers {
		if err := w.WriteEntry(entry); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) == 1 {
		return errs[0]
	}
	if len(errs) > 1 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return fmt.Errorf("logging: multiple writer errors: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// Debug logs a message at DEBUG severity.
func (l *Logger) Debug(scope Scope, msg string, opts ...Option) error {
	return l.log(DEBUG, scope, msg, opts...)
}

// Info logs a message at INFO severity.
func (l *Logger) Info(scope Scope, msg string, opts ...Option) error {
	return l.log(INFO, scope, msg, opts...)
}

// Warn logs a message at WARN severity.
func (l *Logger) Warn(scope Scope, msg string, opts ...Option) error {
	return l.log(WARN, scope, msg, opts...)
}

// Error logs a message at ERROR severity.
func (l *Logger) Error(scope Scope, msg string, opts ...Option) error {
	return l.log(ERROR, scope, msg, opts...)
}

// Logf logs a formatted message at the given severity level.
func (l *Logger) Logf(severity Severity, scope Scope, format string, args ...interface{}) error {
	return l.log(severity, scope, fmt.Sprintf(format, args...))
}

// WithScope returns a ScopedLogger that always uses the given scope,
// reducing boilerplate when logging many entries for the same unit of work.
func (l *Logger) WithScope(scope Scope, opts ...Option) *ScopedLogger {
	return &ScopedLogger{
		logger:   l,
		scope:    scope,
		baseOpts: opts,
	}
}

// ---------------------------------------------------------------------------
// ScopedLogger
// ---------------------------------------------------------------------------

// ScopedLogger is a convenience wrapper around Logger that fixes the scope
// and optional base metadata. All log methods on ScopedLogger use the fixed
// scope and prepend the base options before any per-call options.
type ScopedLogger struct {
	logger   *Logger
	scope    Scope
	baseOpts []Option
}

func (sl *ScopedLogger) mergeOpts(extra []Option) []Option {
	if len(sl.baseOpts) == 0 {
		return extra
	}
	all := make([]Option, 0, len(sl.baseOpts)+len(extra))
	all = append(all, sl.baseOpts...)
	all = append(all, extra...)
	return all
}

// Debug logs a message at DEBUG severity with the fixed scope.
func (sl *ScopedLogger) Debug(msg string, opts ...Option) error {
	return sl.logger.log(DEBUG, sl.scope, msg, sl.mergeOpts(opts)...)
}

// Info logs a message at INFO severity with the fixed scope.
func (sl *ScopedLogger) Info(msg string, opts ...Option) error {
	return sl.logger.log(INFO, sl.scope, msg, sl.mergeOpts(opts)...)
}

// Warn logs a message at WARN severity with the fixed scope.
func (sl *ScopedLogger) Warn(msg string, opts ...Option) error {
	return sl.logger.log(WARN, sl.scope, msg, sl.mergeOpts(opts)...)
}

// Error logs a message at ERROR severity with the fixed scope.
func (sl *ScopedLogger) Error(msg string, opts ...Option) error {
	return sl.logger.log(ERROR, sl.scope, msg, sl.mergeOpts(opts)...)
}

// Scope returns the fixed scope of this ScopedLogger.
func (sl *ScopedLogger) Scope() Scope {
	return sl.scope
}

// ---------------------------------------------------------------------------
// MemoryWriter (for testing)
// ---------------------------------------------------------------------------

// MemoryWriter captures log entries in memory. It is safe for concurrent
// use and intended for testing.
type MemoryWriter struct {
	mu      sync.Mutex
	entries []Entry
}

// NewMemoryWriter creates a new MemoryWriter.
func NewMemoryWriter() *MemoryWriter {
	return &MemoryWriter{}
}

// WriteEntry appends the entry to the in-memory slice.
func (mw *MemoryWriter) WriteEntry(entry Entry) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.entries = append(mw.entries, entry)
	return nil
}

// Entries returns a copy of all captured entries.
func (mw *MemoryWriter) Entries() []Entry {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	cp := make([]Entry, len(mw.entries))
	copy(cp, mw.entries)
	return cp
}

// Len returns the number of captured entries.
func (mw *MemoryWriter) Len() int {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	return len(mw.entries)
}

// Reset clears all captured entries.
func (mw *MemoryWriter) Reset() {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.entries = nil
}

// ---------------------------------------------------------------------------
// ErrorWriter (for testing)
// ---------------------------------------------------------------------------

// ErrorWriter is a Writer that always returns an error. It is intended for
// testing error handling in the Logger.
type ErrorWriter struct {
	Err error
}

// WriteEntry always returns the configured error.
func (ew *ErrorWriter) WriteEntry(_ Entry) error {
	return ew.Err
}
