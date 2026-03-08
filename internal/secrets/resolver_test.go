package secrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers for resolver tests
// ---------------------------------------------------------------------------

// fakeEnv returns an EnvLookupFunc backed by a map. Variables not in the
// map behave as unset.
func fakeEnv(vars map[string]string) EnvLookupFunc {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

// emptyEnv returns an EnvLookupFunc where no variables are set.
func emptyEnv() EnvLookupFunc {
	return fakeEnv(nil)
}

// fakeFS returns a FileReadFunc backed by a map of path → content.
// Paths not in the map return os.ErrNotExist.
func fakeFS(files map[string]string) FileReadFunc {
	return func(name string) ([]byte, error) {
		content, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("open %s: no such file or directory", name)
		}
		return []byte(content), nil
	}
}

// emptyFS returns a FileReadFunc where no files exist.
func emptyFS() FileReadFunc {
	return fakeFS(nil)
}

// failFS returns a FileReadFunc that always returns the given error.
func failFS(err error) FileReadFunc {
	return func(name string) ([]byte, error) {
		return nil, err
	}
}

// resolverEncryptor creates an Encryptor for resolver tests.
func resolverEncryptor(t *testing.T) *Encryptor {
	t.Helper()
	key := base64.StdEncoding.EncodeToString([]byte("resolver-test-key-32-bytes-long!"))
	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("failed to create resolver test encryptor: %v", err)
	}
	t.Cleanup(func() { enc.Close() })
	return enc
}

// resolverStoreWithCred creates an InMemoryCredentialStore with one generic
// credential already stored.
func resolverStoreWithCred(t *testing.T, name, value string) *InMemoryCredentialStore {
	t.Helper()
	store := NewInMemoryCredentialStore(resolverEncryptor(t))
	_, err := store.Create(context.Background(), CreateCredentialInput{
		Name:           name,
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte(value),
		CreatedBy:      "test",
	})
	if err != nil {
		t.Fatalf("failed to seed resolver store: %v", err)
	}
	return store
}

// ---------------------------------------------------------------------------
// Resolve — database source (highest precedence)
// ---------------------------------------------------------------------------

func TestResolver_Resolve_Database_Success(t *testing.T) {
	store := resolverStoreWithCred(t, "my-db-cred", "db-secret-value")
	resolver := NewCredentialResolver(store,
		WithEnvLookup(emptyEnv()),
		WithFileRead(emptyFS()),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "my-db-cred",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "db-secret-value" {
		t.Errorf("Plaintext = %q, want %q", string(rc.Plaintext), "db-secret-value")
	}
	if rc.Source != SourceDatabase {
		t.Errorf("Source = %q, want %q", rc.Source, SourceDatabase)
	}
	if rc.SourceDetail != "my-db-cred" {
		t.Errorf("SourceDetail = %q, want %q", rc.SourceDetail, "my-db-cred")
	}
}

func TestResolver_Resolve_Database_NotFound(t *testing.T) {
	store := resolverStoreWithCred(t, "existing", "value")
	resolver := NewCredentialResolver(store,
		WithEnvLookup(emptyEnv()),
		WithFileRead(emptyFS()),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for missing DB credential")
	}
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound in chain, got: %v", err)
	}
}

func TestResolver_Resolve_Database_NilStore(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(emptyFS()),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "some-cred",
	})
	if err == nil {
		t.Fatal("expected error when store is nil")
	}
	if !strings.Contains(err.Error(), "credential store is not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolver_Resolve_Database_StoreNoEncryptor(t *testing.T) {
	store := NewInMemoryCredentialStore(nil) // no encryptor
	resolver := NewCredentialResolver(store,
		WithEnvLookup(emptyEnv()),
		WithFileRead(emptyFS()),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "anything",
	})
	if err == nil {
		t.Fatal("expected error when encryptor is nil")
	}
	if !errors.Is(err, ErrEncryptionKeyNotConfigured) {
		t.Errorf("expected ErrEncryptionKeyNotConfigured in chain, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Resolve — environment variable source (second precedence)
// ---------------------------------------------------------------------------

func TestResolver_Resolve_EnvVar_Success(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(fakeEnv(map[string]string{
			"MY_SECRET_VAR": "env-secret-value",
		})),
		WithFileRead(emptyFS()),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		EnvVar: "MY_SECRET_VAR",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "env-secret-value" {
		t.Errorf("Plaintext = %q, want %q", string(rc.Plaintext), "env-secret-value")
	}
	if rc.Source != SourceEnvVar {
		t.Errorf("Source = %q, want %q", rc.Source, SourceEnvVar)
	}
	if rc.SourceDetail != "MY_SECRET_VAR" {
		t.Errorf("SourceDetail = %q, want %q", rc.SourceDetail, "MY_SECRET_VAR")
	}
}

func TestResolver_Resolve_EnvVar_NotSet(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(emptyFS()),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		EnvVar: "MISSING_VAR",
	})
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
	if !errors.Is(err, ErrEnvVarNotSet) {
		t.Errorf("expected ErrEnvVarNotSet, got: %v", err)
	}
	if !strings.Contains(err.Error(), "MISSING_VAR") {
		t.Errorf("error should mention variable name, got: %v", err)
	}
}

func TestResolver_Resolve_EnvVar_Empty(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(fakeEnv(map[string]string{
			"EMPTY_VAR": "",
		})),
		WithFileRead(emptyFS()),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		EnvVar: "EMPTY_VAR",
	})
	if err == nil {
		t.Fatal("expected error for empty env var")
	}
	if !errors.Is(err, ErrEnvVarNotSet) {
		t.Errorf("expected ErrEnvVarNotSet, got: %v", err)
	}
}

func TestResolver_Resolve_EnvVar_SpecialCharacters(t *testing.T) {
	specialVal := "p@$$w0rd!#%^&*()\n\ttab"
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(fakeEnv(map[string]string{
			"SPECIAL": specialVal,
		})),
		WithFileRead(emptyFS()),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		EnvVar: "SPECIAL",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != specialVal {
		t.Errorf("Plaintext = %q, want %q", string(rc.Plaintext), specialVal)
	}
}

func TestResolver_Resolve_EnvVar_Unicode(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(fakeEnv(map[string]string{
			"UNICODE_VAR": "パスワード🔑",
		})),
		WithFileRead(emptyFS()),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		EnvVar: "UNICODE_VAR",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "パスワード🔑" {
		t.Errorf("Plaintext = %q", string(rc.Plaintext))
	}
}

// ---------------------------------------------------------------------------
// Resolve — file path source (lowest precedence)
// ---------------------------------------------------------------------------

func TestResolver_Resolve_File_Success(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{
			"/etc/keys/my.pem": "file-secret-value",
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		FilePath: "/etc/keys/my.pem",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "file-secret-value" {
		t.Errorf("Plaintext = %q, want %q", string(rc.Plaintext), "file-secret-value")
	}
	if rc.Source != SourceFile {
		t.Errorf("Source = %q, want %q", rc.Source, SourceFile)
	}
	if rc.SourceDetail != "/etc/keys/my.pem" {
		t.Errorf("SourceDetail = %q, want %q", rc.SourceDetail, "/etc/keys/my.pem")
	}
}

func TestResolver_Resolve_File_NotFound(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(emptyFS()),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		FilePath: "/nonexistent/key.pem",
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, ErrFileNotReadable) {
		t.Errorf("expected ErrFileNotReadable, got: %v", err)
	}
	if !strings.Contains(err.Error(), "/nonexistent/key.pem") {
		t.Errorf("error should mention file path, got: %v", err)
	}
}

func TestResolver_Resolve_File_Empty(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{
			"/empty.pem": "",
		})),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		FilePath: "/empty.pem",
	})
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !errors.Is(err, ErrFileNotReadable) {
		t.Errorf("expected ErrFileNotReadable, got: %v", err)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

func TestResolver_Resolve_File_OnlyNewlines(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{
			"/whitespace.pem": "\n\n\r\n",
		})),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		FilePath: "/whitespace.pem",
	})
	if err == nil {
		t.Fatal("expected error for file containing only newlines")
	}
	if !errors.Is(err, ErrFileNotReadable) {
		t.Errorf("expected ErrFileNotReadable, got: %v", err)
	}
	if !strings.Contains(err.Error(), "whitespace") {
		t.Errorf("error should mention 'whitespace', got: %v", err)
	}
}

func TestResolver_Resolve_File_TrailingNewlineStripped(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{
			"/key.pem": "my-secret-key\n",
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		FilePath: "/key.pem",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "my-secret-key" {
		t.Errorf("Plaintext = %q, want %q (trailing newline should be stripped)", string(rc.Plaintext), "my-secret-key")
	}
}

func TestResolver_Resolve_File_TrailingCRLFStripped(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{
			"/key.pem": "windows-secret\r\n",
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		FilePath: "/key.pem",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "windows-secret" {
		t.Errorf("Plaintext = %q, want %q", string(rc.Plaintext), "windows-secret")
	}
}

func TestResolver_Resolve_File_MultipleTrailingNewlinesStripped(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{
			"/key.pem": "secret\n\n\n",
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		FilePath: "/key.pem",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "secret" {
		t.Errorf("Plaintext = %q, want %q", string(rc.Plaintext), "secret")
	}
}

func TestResolver_Resolve_File_InternalNewlinesPreserved(t *testing.T) {
	pemContent := "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\nmore...\n-----END RSA PRIVATE KEY-----\n"
	expected := "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\nmore...\n-----END RSA PRIVATE KEY-----"

	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{
			"/pem.key": pemContent,
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		FilePath: "/pem.key",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != expected {
		t.Errorf("internal newlines were not preserved:\ngot:  %q\nwant: %q", string(rc.Plaintext), expected)
	}
}

func TestResolver_Resolve_File_LeadingWhitespacePreserved(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{
			"/key.pem": "  leading-spaces-secret\n",
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		FilePath: "/key.pem",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "  leading-spaces-secret" {
		t.Errorf("Plaintext = %q, leading whitespace should be preserved", string(rc.Plaintext))
	}
}

func TestResolver_Resolve_File_ReadError(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(failFS(fmt.Errorf("permission denied"))),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		FilePath: "/protected/key.pem",
	})
	if err == nil {
		t.Fatal("expected error for read failure")
	}
	if !errors.Is(err, ErrFileNotReadable) {
		t.Errorf("expected ErrFileNotReadable, got: %v", err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should mention the underlying cause, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Resolve — no sources configured
// ---------------------------------------------------------------------------

func TestResolver_Resolve_NoSourcesConfigured(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(emptyFS()),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{})
	if err == nil {
		t.Fatal("expected error for empty source")
	}
	if !errors.Is(err, ErrNoCredentialConfigured) {
		t.Errorf("expected ErrNoCredentialConfigured, got: %v", err)
	}
}

func TestResolver_Resolve_AllFieldsEmpty(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(emptyFS()),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "",
		EnvVar:         "",
		FilePath:       "",
	})
	if !errors.Is(err, ErrNoCredentialConfigured) {
		t.Errorf("expected ErrNoCredentialConfigured, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Resolve — precedence ordering
// ---------------------------------------------------------------------------

func TestResolver_Resolve_Precedence_DatabaseBeatsEnvVar(t *testing.T) {
	store := resolverStoreWithCred(t, "db-cred", "db-value")
	resolver := NewCredentialResolver(store,
		WithEnvLookup(fakeEnv(map[string]string{
			"ENV_CRED": "env-value",
		})),
		WithFileRead(emptyFS()),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "db-cred",
		EnvVar:         "ENV_CRED",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "db-value" {
		t.Errorf("Plaintext = %q, want %q (database should win over env var)", string(rc.Plaintext), "db-value")
	}
	if rc.Source != SourceDatabase {
		t.Errorf("Source = %q, want %q", rc.Source, SourceDatabase)
	}
}

func TestResolver_Resolve_Precedence_DatabaseBeatsFile(t *testing.T) {
	store := resolverStoreWithCred(t, "db-cred", "db-value")
	resolver := NewCredentialResolver(store,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{
			"/path/key.pem": "file-value",
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "db-cred",
		FilePath:       "/path/key.pem",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "db-value" {
		t.Errorf("Plaintext = %q, want %q (database should win over file)", string(rc.Plaintext), "db-value")
	}
	if rc.Source != SourceDatabase {
		t.Errorf("Source = %q, want %q", rc.Source, SourceDatabase)
	}
}

func TestResolver_Resolve_Precedence_EnvVarBeatsFile(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(fakeEnv(map[string]string{
			"ENV_CRED": "env-value",
		})),
		WithFileRead(fakeFS(map[string]string{
			"/path/key.pem": "file-value",
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		EnvVar:   "ENV_CRED",
		FilePath: "/path/key.pem",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "env-value" {
		t.Errorf("Plaintext = %q, want %q (env var should win over file)", string(rc.Plaintext), "env-value")
	}
	if rc.Source != SourceEnvVar {
		t.Errorf("Source = %q, want %q", rc.Source, SourceEnvVar)
	}
}

func TestResolver_Resolve_Precedence_DatabaseBeatsAll(t *testing.T) {
	store := resolverStoreWithCred(t, "db-cred", "db-value")
	resolver := NewCredentialResolver(store,
		WithEnvLookup(fakeEnv(map[string]string{
			"ENV_CRED": "env-value",
		})),
		WithFileRead(fakeFS(map[string]string{
			"/path/key.pem": "file-value",
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "db-cred",
		EnvVar:         "ENV_CRED",
		FilePath:       "/path/key.pem",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "db-value" {
		t.Errorf("Plaintext = %q, want %q (database should win over all)", string(rc.Plaintext), "db-value")
	}
	if rc.Source != SourceDatabase {
		t.Errorf("Source = %q, want %q", rc.Source, SourceDatabase)
	}
}

func TestResolver_Resolve_Precedence_FallsToEnvWhenDBNameEmpty(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(fakeEnv(map[string]string{
			"FALLBACK_VAR": "fallback-env-value",
		})),
		WithFileRead(fakeFS(map[string]string{
			"/fallback.pem": "fallback-file-value",
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "", // empty — skip DB
		EnvVar:         "FALLBACK_VAR",
		FilePath:       "/fallback.pem",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "fallback-env-value" {
		t.Errorf("should fall through to env var when DB name is empty, got %q", string(rc.Plaintext))
	}
	if rc.Source != SourceEnvVar {
		t.Errorf("Source = %q, want %q", rc.Source, SourceEnvVar)
	}
}

func TestResolver_Resolve_Precedence_FallsToFileWhenDBAndEnvEmpty(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{
			"/last-resort.pem": "file-only-value",
		})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "",
		EnvVar:         "",
		FilePath:       "/last-resort.pem",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if string(rc.Plaintext) != "file-only-value" {
		t.Errorf("should fall through to file, got %q", string(rc.Plaintext))
	}
	if rc.Source != SourceFile {
		t.Errorf("Source = %q, want %q", rc.Source, SourceFile)
	}
}

// ---------------------------------------------------------------------------
// Resolve — no fallthrough on error (spec: if highest-precedence source
// is configured but fails, the error is returned immediately)
// ---------------------------------------------------------------------------

func TestResolver_Resolve_NoFallthrough_DBErrorDoesNotFallToEnv(t *testing.T) {
	store := resolverStoreWithCred(t, "existing", "value")
	resolver := NewCredentialResolver(store,
		WithEnvLookup(fakeEnv(map[string]string{
			"BACKUP": "backup-value",
		})),
		WithFileRead(emptyFS()),
	)

	// DB credential name is set but points to a nonexistent credential.
	_, err := resolver.Resolve(context.Background(), CredentialSource{
		CredentialName: "nonexistent-db-cred",
		EnvVar:         "BACKUP",
	})
	if err == nil {
		t.Fatal("expected error — should NOT fall through to env var")
	}
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound, got: %v", err)
	}
}

func TestResolver_Resolve_NoFallthrough_EnvErrorDoesNotFallToFile(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()), // MISSING_VAR is not set
		WithFileRead(fakeFS(map[string]string{
			"/backup.pem": "backup-file-value",
		})),
	)

	_, err := resolver.Resolve(context.Background(), CredentialSource{
		EnvVar:   "MISSING_VAR",
		FilePath: "/backup.pem",
	})
	if err == nil {
		t.Fatal("expected error — should NOT fall through to file")
	}
	if !errors.Is(err, ErrEnvVarNotSet) {
		t.Errorf("expected ErrEnvVarNotSet, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Resolve — plaintext can be zeroed
// ---------------------------------------------------------------------------

func TestResolver_Resolve_PlaintextCanBeZeroed(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(fakeEnv(map[string]string{
			"ZERO_ME": "sensitive-material",
		})),
		WithFileRead(emptyFS()),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{
		EnvVar: "ZERO_ME",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if len(rc.Plaintext) == 0 {
		t.Fatal("Plaintext should not be empty")
	}

	ZeroBytes(rc.Plaintext)

	if !IsZeroed(rc.Plaintext) {
		t.Error("Plaintext should be zeroed after ZeroBytes")
	}
}

// ---------------------------------------------------------------------------
// IsConfigured
// ---------------------------------------------------------------------------

func TestResolver_IsConfigured_AllEmpty(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	if resolver.IsConfigured(CredentialSource{}) {
		t.Error("empty source should not be configured")
	}
}

func TestResolver_IsConfigured_CredentialNameOnly(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	if !resolver.IsConfigured(CredentialSource{CredentialName: "cred"}) {
		t.Error("CredentialName should count as configured")
	}
}

func TestResolver_IsConfigured_EnvVarOnly(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	if !resolver.IsConfigured(CredentialSource{EnvVar: "VAR"}) {
		t.Error("EnvVar should count as configured")
	}
}

func TestResolver_IsConfigured_FilePathOnly(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	if !resolver.IsConfigured(CredentialSource{FilePath: "/path"}) {
		t.Error("FilePath should count as configured")
	}
}

func TestResolver_IsConfigured_AllSet(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	src := CredentialSource{
		CredentialName: "cred",
		EnvVar:         "VAR",
		FilePath:       "/path",
	}
	if !resolver.IsConfigured(src) {
		t.Error("all fields set should be configured")
	}
}

func TestResolver_IsConfigured_CombinationEnvAndFile(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	src := CredentialSource{
		EnvVar:   "VAR",
		FilePath: "/path",
	}
	if !resolver.IsConfigured(src) {
		t.Error("env + file should be configured")
	}
}

// ---------------------------------------------------------------------------
// DescribeSource
// ---------------------------------------------------------------------------

func TestResolver_DescribeSource_Database(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	desc := resolver.DescribeSource(CredentialSource{
		CredentialName: "my-cred",
		EnvVar:         "ALSO_SET",
		FilePath:       "/also/set.pem",
	})
	if !strings.Contains(desc, "database") {
		t.Errorf("should describe database source, got %q", desc)
	}
	if !strings.Contains(desc, "my-cred") {
		t.Errorf("should mention credential name, got %q", desc)
	}
}

func TestResolver_DescribeSource_EnvVar(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	desc := resolver.DescribeSource(CredentialSource{
		EnvVar:   "MY_VAR",
		FilePath: "/also/set.pem",
	})
	if !strings.Contains(desc, "environment variable") {
		t.Errorf("should describe env var source, got %q", desc)
	}
	if !strings.Contains(desc, "MY_VAR") {
		t.Errorf("should mention variable name, got %q", desc)
	}
}

func TestResolver_DescribeSource_File(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	desc := resolver.DescribeSource(CredentialSource{
		FilePath: "/etc/keys/my.pem",
	})
	if !strings.Contains(desc, "file") {
		t.Errorf("should describe file source, got %q", desc)
	}
	if !strings.Contains(desc, "/etc/keys/my.pem") {
		t.Errorf("should mention file path, got %q", desc)
	}
}

func TestResolver_DescribeSource_None(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	desc := resolver.DescribeSource(CredentialSource{})
	if desc != "none" {
		t.Errorf("should describe as 'none', got %q", desc)
	}
}

func TestResolver_DescribeSource_PrecedenceReflected(t *testing.T) {
	resolver := NewCredentialResolver(nil)

	// When all three are set, DescribeSource should report the highest
	// precedence (database).
	desc := resolver.DescribeSource(CredentialSource{
		CredentialName: "db-cred",
		EnvVar:         "ENV_VAR",
		FilePath:       "/file.pem",
	})
	if !strings.Contains(desc, "database") {
		t.Errorf("should reflect database as highest precedence, got %q", desc)
	}

	// When only env and file are set, should report env.
	desc = resolver.DescribeSource(CredentialSource{
		EnvVar:   "ENV_VAR",
		FilePath: "/file.pem",
	})
	if !strings.Contains(desc, "environment") {
		t.Errorf("should reflect env var when no DB, got %q", desc)
	}
}

// ---------------------------------------------------------------------------
// Constructor option tests
// ---------------------------------------------------------------------------

func TestNewCredentialResolver_Defaults(t *testing.T) {
	resolver := NewCredentialResolver(nil)
	if resolver == nil {
		t.Fatal("NewCredentialResolver should not return nil")
	}
	if resolver.envFunc == nil {
		t.Error("envFunc should default to non-nil (os.LookupEnv)")
	}
	if resolver.readFunc == nil {
		t.Error("readFunc should default to non-nil (os.ReadFile)")
	}
}

func TestNewCredentialResolver_WithNilOptions(t *testing.T) {
	// Passing nil function options should not replace the defaults.
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(nil),
		WithFileRead(nil),
	)
	if resolver.envFunc == nil {
		t.Error("envFunc should remain default when nil is passed")
	}
	if resolver.readFunc == nil {
		t.Error("readFunc should remain default when nil is passed")
	}
}

func TestNewCredentialResolver_WithStore(t *testing.T) {
	store := resolverStoreWithCred(t, "test", "val")
	resolver := NewCredentialResolver(store)
	if resolver.store != store {
		t.Error("store should be stored in resolver")
	}
}

// ---------------------------------------------------------------------------
// CredentialSourceKind constants
// ---------------------------------------------------------------------------

func TestCredentialSourceKind_Values(t *testing.T) {
	if SourceDatabase != "database" {
		t.Errorf("SourceDatabase = %q", SourceDatabase)
	}
	if SourceEnvVar != "env" {
		t.Errorf("SourceEnvVar = %q", SourceEnvVar)
	}
	if SourceFile != "file" {
		t.Errorf("SourceFile = %q", SourceFile)
	}
}

// ---------------------------------------------------------------------------
// Sentinel error identity tests
// ---------------------------------------------------------------------------

func TestResolverSentinelErrors_AreDistinct(t *testing.T) {
	sentinels := []error{
		ErrNoCredentialConfigured,
		ErrEnvVarNotSet,
		ErrFileNotReadable,
	}

	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			if errors.Is(sentinels[i], sentinels[j]) {
				t.Errorf("sentinel errors should be distinct: %v == %v", sentinels[i], sentinels[j])
			}
		}
	}
}

func TestResolverSentinelErrors_HaveSecretsPrefix(t *testing.T) {
	sentinels := map[string]error{
		"ErrNoCredentialConfigured": ErrNoCredentialConfigured,
		"ErrEnvVarNotSet":           ErrEnvVarNotSet,
		"ErrFileNotReadable":        ErrFileNotReadable,
	}
	for name, err := range sentinels {
		if !strings.HasPrefix(err.Error(), "secrets:") {
			t.Errorf("%s should start with 'secrets:', got %q", name, err.Error())
		}
	}
}

// ---------------------------------------------------------------------------
// Context propagation
// ---------------------------------------------------------------------------

func TestResolver_Resolve_CancelledContext_Database(t *testing.T) {
	store := resolverStoreWithCred(t, "ctx-test", "value")
	resolver := NewCredentialResolver(store,
		WithEnvLookup(emptyEnv()),
		WithFileRead(emptyFS()),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The InMemoryCredentialStore doesn't check context, so this succeeds.
	// For a real DB-backed store, it would fail with context.Canceled.
	// This test just verifies the context is passed through without panicking.
	_, err := resolver.Resolve(ctx, CredentialSource{
		CredentialName: "ctx-test",
	})
	// In-memory store ignores context, so may succeed or fail.
	_ = err
}

// ---------------------------------------------------------------------------
// Multiple sequential resolves
// ---------------------------------------------------------------------------

func TestResolver_Resolve_MultipleSequentialCalls(t *testing.T) {
	store := resolverStoreWithCred(t, "multi-cred", "multi-value")
	resolver := NewCredentialResolver(store,
		WithEnvLookup(fakeEnv(map[string]string{
			"MULTI_ENV": "multi-env-value",
		})),
		WithFileRead(fakeFS(map[string]string{
			"/multi.pem": "multi-file-value",
		})),
	)

	ctx := context.Background()

	// Resolve from DB.
	rc1, err := resolver.Resolve(ctx, CredentialSource{CredentialName: "multi-cred"})
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	if string(rc1.Plaintext) != "multi-value" {
		t.Errorf("first resolve = %q", string(rc1.Plaintext))
	}
	ZeroBytes(rc1.Plaintext)

	// Resolve from env var.
	rc2, err := resolver.Resolve(ctx, CredentialSource{EnvVar: "MULTI_ENV"})
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if string(rc2.Plaintext) != "multi-env-value" {
		t.Errorf("second resolve = %q", string(rc2.Plaintext))
	}
	ZeroBytes(rc2.Plaintext)

	// Resolve from file.
	rc3, err := resolver.Resolve(ctx, CredentialSource{FilePath: "/multi.pem"})
	if err != nil {
		t.Fatalf("third resolve failed: %v", err)
	}
	if string(rc3.Plaintext) != "multi-file-value" {
		t.Errorf("third resolve = %q", string(rc3.Plaintext))
	}
	ZeroBytes(rc3.Plaintext)
}

// ---------------------------------------------------------------------------
// Resolve — no caching (each call re-reads source)
// ---------------------------------------------------------------------------

func TestResolver_Resolve_NoCaching_EnvVarChanges(t *testing.T) {
	currentValue := "initial-value"
	dynamicEnv := func(key string) (string, bool) {
		if key == "DYNAMIC" {
			return currentValue, true
		}
		return "", false
	}

	resolver := NewCredentialResolver(nil,
		WithEnvLookup(dynamicEnv),
		WithFileRead(emptyFS()),
	)

	ctx := context.Background()

	rc, err := resolver.Resolve(ctx, CredentialSource{EnvVar: "DYNAMIC"})
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	if string(rc.Plaintext) != "initial-value" {
		t.Errorf("first resolve = %q", string(rc.Plaintext))
	}
	ZeroBytes(rc.Plaintext)

	// Change the "env var".
	currentValue = "updated-value"

	rc, err = resolver.Resolve(ctx, CredentialSource{EnvVar: "DYNAMIC"})
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if string(rc.Plaintext) != "updated-value" {
		t.Errorf("second resolve = %q, resolver should not cache", string(rc.Plaintext))
	}
	ZeroBytes(rc.Plaintext)
}

func TestResolver_Resolve_NoCaching_FileChanges(t *testing.T) {
	currentContent := "version-1"
	dynamicFS := func(name string) ([]byte, error) {
		if name == "/changing.pem" {
			return []byte(currentContent), nil
		}
		return nil, fmt.Errorf("not found")
	}

	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(dynamicFS),
	)

	ctx := context.Background()

	rc, err := resolver.Resolve(ctx, CredentialSource{FilePath: "/changing.pem"})
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	if string(rc.Plaintext) != "version-1" {
		t.Errorf("first resolve = %q", string(rc.Plaintext))
	}
	ZeroBytes(rc.Plaintext)

	currentContent = "version-2"

	rc, err = resolver.Resolve(ctx, CredentialSource{FilePath: "/changing.pem"})
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if string(rc.Plaintext) != "version-2" {
		t.Errorf("second resolve = %q, resolver should not cache", string(rc.Plaintext))
	}
	ZeroBytes(rc.Plaintext)
}

// ---------------------------------------------------------------------------
// ResolvedCredential struct field tests
// ---------------------------------------------------------------------------

func TestResolvedCredential_SourceDetail_Database(t *testing.T) {
	store := resolverStoreWithCred(t, "detail-test", "val")
	resolver := NewCredentialResolver(store, WithEnvLookup(emptyEnv()), WithFileRead(emptyFS()))

	rc, err := resolver.Resolve(context.Background(), CredentialSource{CredentialName: "detail-test"})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if rc.SourceDetail != "detail-test" {
		t.Errorf("SourceDetail = %q, want %q", rc.SourceDetail, "detail-test")
	}
}

func TestResolvedCredential_SourceDetail_EnvVar(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(fakeEnv(map[string]string{"DETAIL_VAR": "x"})),
		WithFileRead(emptyFS()),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{EnvVar: "DETAIL_VAR"})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if rc.SourceDetail != "DETAIL_VAR" {
		t.Errorf("SourceDetail = %q, want %q", rc.SourceDetail, "DETAIL_VAR")
	}
}

func TestResolvedCredential_SourceDetail_File(t *testing.T) {
	resolver := NewCredentialResolver(nil,
		WithEnvLookup(emptyEnv()),
		WithFileRead(fakeFS(map[string]string{"/detail/path.pem": "x"})),
	)

	rc, err := resolver.Resolve(context.Background(), CredentialSource{FilePath: "/detail/path.pem"})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer ZeroBytes(rc.Plaintext)

	if rc.SourceDetail != "/detail/path.pem" {
		t.Errorf("SourceDetail = %q, want %q", rc.SourceDetail, "/detail/path.pem")
	}
}
