package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Sentinel errors returned by credential resolution.
var (
	// ErrNoCredentialConfigured is returned when none of the three
	// credential sources (database, environment variable, file path) are
	// configured for a given entity.
	ErrNoCredentialConfigured = errors.New("secrets: no credential source configured")

	// ErrEnvVarNotSet is returned when the configured environment variable
	// exists in the source spec but the variable is empty or unset in the
	// process environment.
	ErrEnvVarNotSet = errors.New("secrets: environment variable is not set")

	// ErrFileNotReadable is returned when the configured file path cannot
	// be read (does not exist, permission denied, etc.).
	ErrFileNotReadable = errors.New("secrets: credential file is not readable")
)

// CredentialSourceKind describes which resolution source provided a
// credential value.
type CredentialSourceKind string

const (
	// SourceDatabase indicates the credential was decrypted from the
	// credentials table via the CredentialStore.
	SourceDatabase CredentialSourceKind = "database"

	// SourceEnvVar indicates the credential was read from a process
	// environment variable.
	SourceEnvVar CredentialSourceKind = "env"

	// SourceFile indicates the credential was read from a file on disk.
	SourceFile CredentialSourceKind = "file"
)

// CredentialSource describes the configured credential sources for a single
// entity (e.g. one Chef organisation, the LDAP provider, the SMTP sender).
// The resolver walks the fields in precedence order:
//
//  1. CredentialName (database)  →  2. EnvVar  →  3. FilePath
//
// Only the first non-empty source is used. All three may be empty, in which
// case resolution returns ErrNoCredentialConfigured.
type CredentialSource struct {
	// CredentialName is the name of a credential stored in the database
	// (credentials.name). If non-empty, this is tried first.
	CredentialName string

	// EnvVar is the name of an environment variable that holds the
	// credential value. Tried second if CredentialName is empty.
	EnvVar string

	// FilePath is an absolute or relative path to a file containing the
	// credential value (e.g. a PEM key file). Tried last.
	FilePath string
}

// ResolvedCredential holds the plaintext credential value together with
// metadata about where it came from. The caller MUST call
// ZeroBytes(rc.Plaintext) when the value is no longer needed.
type ResolvedCredential struct {
	// Plaintext is the raw credential value. The caller MUST zero this
	// slice after use.
	Plaintext []byte

	// Source indicates which resolution method produced the value.
	Source CredentialSourceKind

	// SourceDetail provides human-readable context about the source:
	//   - SourceDatabase: the credential name
	//   - SourceEnvVar:   the environment variable name
	//   - SourceFile:     the file path
	SourceDetail string
}

// EnvLookupFunc is the signature for a function that looks up an
// environment variable by name. It returns the value and a boolean
// indicating whether the variable was set. This matches os.LookupEnv
// and allows tests to inject a fake environment.
type EnvLookupFunc func(key string) (string, bool)

// FileReadFunc is the signature for a function that reads a file and
// returns its contents. It matches the signature of os.ReadFile and
// allows tests to inject a fake filesystem.
type FileReadFunc func(name string) ([]byte, error)

// CredentialResolver resolves credential values from one of three sources
// in precedence order: database → environment variable → file path.
//
// It is safe for concurrent use. The resolver does not cache results —
// each call re-reads the underlying source.
type CredentialResolver struct {
	store    CredentialStore
	envFunc  EnvLookupFunc
	readFunc FileReadFunc
}

// NewCredentialResolver creates a resolver. The store may be nil if no
// database-backed credentials are in use (database resolution will be
// skipped, and if a CredentialSource specifies a CredentialName, the
// resolver will return ErrEncryptionKeyNotConfigured or
// ErrCredentialNotFound).
//
// By default the resolver reads real environment variables and real files.
// Use ResolverOption functions to inject test doubles.
func NewCredentialResolver(store CredentialStore, opts ...ResolverOption) *CredentialResolver {
	r := &CredentialResolver{
		store:    store,
		envFunc:  os.LookupEnv,
		readFunc: os.ReadFile,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ResolverOption configures optional behaviour on a CredentialResolver.
type ResolverOption func(*CredentialResolver)

// WithEnvLookup replaces the default os.LookupEnv with a custom function.
// This is intended for testing.
func WithEnvLookup(fn EnvLookupFunc) ResolverOption {
	return func(r *CredentialResolver) {
		if fn != nil {
			r.envFunc = fn
		}
	}
}

// WithFileRead replaces the default os.ReadFile with a custom function.
// This is intended for testing.
func WithFileRead(fn FileReadFunc) ResolverOption {
	return func(r *CredentialResolver) {
		if fn != nil {
			r.readFunc = fn
		}
	}
}

// Resolve resolves a credential value from the configured sources in
// precedence order: database → environment variable → file path.
//
// The returned ResolvedCredential contains the plaintext value. The caller
// MUST call ZeroBytes(rc.Plaintext) when the value is no longer needed.
//
// If the highest-precedence source is configured but fails (e.g. the
// database credential cannot be decrypted, or the env var is empty), the
// error is returned immediately — the resolver does NOT fall through to
// lower-precedence sources. This is intentional: if an operator configures
// a database credential, a missing or broken database credential is an
// error, not a cue to silently fall back to a file.
func (r *CredentialResolver) Resolve(ctx context.Context, src CredentialSource) (*ResolvedCredential, error) {
	// 1. Database credential (highest precedence).
	if src.CredentialName != "" {
		return r.resolveFromDatabase(ctx, src.CredentialName)
	}

	// 2. Environment variable.
	if src.EnvVar != "" {
		return r.resolveFromEnvVar(src.EnvVar)
	}

	// 3. File path (lowest precedence).
	if src.FilePath != "" {
		return r.resolveFromFile(src.FilePath)
	}

	return nil, ErrNoCredentialConfigured
}

// IsConfigured returns true if at least one credential source is specified
// in the given CredentialSource. This is useful for startup validation to
// detect entities that have no credential configured at all.
func (r *CredentialResolver) IsConfigured(src CredentialSource) bool {
	return src.CredentialName != "" || src.EnvVar != "" || src.FilePath != ""
}

// DescribeSource returns a human-readable description of which source
// would be used for the given CredentialSource, without actually resolving
// the credential. This is useful for logging and diagnostics.
func (r *CredentialResolver) DescribeSource(src CredentialSource) string {
	if src.CredentialName != "" {
		return fmt.Sprintf("database credential %q", src.CredentialName)
	}
	if src.EnvVar != "" {
		return fmt.Sprintf("environment variable %q", src.EnvVar)
	}
	if src.FilePath != "" {
		return fmt.Sprintf("file %q", src.FilePath)
	}
	return "none"
}

// resolveFromDatabase decrypts a credential from the CredentialStore.
func (r *CredentialResolver) resolveFromDatabase(ctx context.Context, credentialName string) (*ResolvedCredential, error) {
	if r.store == nil {
		return nil, fmt.Errorf("secrets: cannot resolve database credential %q: credential store is not available", credentialName)
	}

	cred, err := r.store.Get(ctx, credentialName)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to resolve database credential %q: %w", credentialName, err)
	}

	return &ResolvedCredential{
		Plaintext:    cred.Plaintext,
		Source:       SourceDatabase,
		SourceDetail: credentialName,
	}, nil
}

// resolveFromEnvVar reads a credential from a process environment variable.
func (r *CredentialResolver) resolveFromEnvVar(envVar string) (*ResolvedCredential, error) {
	value, ok := r.envFunc(envVar)
	if !ok || value == "" {
		return nil, fmt.Errorf("%w: %s", ErrEnvVarNotSet, envVar)
	}

	// Copy the string into a byte slice so the caller can zero it.
	plaintext := []byte(value)

	return &ResolvedCredential{
		Plaintext:    plaintext,
		Source:       SourceEnvVar,
		SourceDetail: envVar,
	}, nil
}

// resolveFromFile reads a credential from a file on disk. Trailing
// newlines are stripped to handle common PEM and password file formats
// where editors append a final newline.
func (r *CredentialResolver) resolveFromFile(filePath string) (*ResolvedCredential, error) {
	data, err := r.readFunc(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrFileNotReadable, filePath, err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("%w: %s: file is empty", ErrFileNotReadable, filePath)
	}

	// Trim trailing newlines/carriage returns only. Leading whitespace and
	// internal newlines (common in PEM files) are preserved.
	plaintext := []byte(strings.TrimRight(string(data), "\r\n"))

	if len(plaintext) == 0 {
		return nil, fmt.Errorf("%w: %s: file contains only whitespace", ErrFileNotReadable, filePath)
	}

	return &ResolvedCredential{
		Plaintext:    plaintext,
		Source:       SourceFile,
		SourceDetail: filePath,
	}, nil
}
