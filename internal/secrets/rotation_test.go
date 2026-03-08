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
// Test helpers for rotation tests
// ---------------------------------------------------------------------------

// rotationKey generates a distinct Base64-encoded 32-byte key for rotation
// tests. The index parameter produces a different key each time.
func rotationKey(t *testing.T, index int) string {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte((index*37 + i*13) % 256)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func rotationEncryptor(t *testing.T, index int) *Encryptor {
	t.Helper()
	enc, err := NewEncryptor(rotationKey(t, index))
	if err != nil {
		t.Fatalf("failed to create rotation encryptor %d: %v", index, err)
	}
	t.Cleanup(func() { enc.Close() })
	return enc
}

// encryptRow is a helper that encrypts a plaintext value with the given
// encryptor and returns a RotationRow ready for rotation testing.
func encryptRow(t *testing.T, enc *Encryptor, name, credType, plaintext string) RotationRow {
	t.Helper()
	aad, err := BuildAAD(credType, name)
	if err != nil {
		t.Fatalf("BuildAAD failed: %v", err)
	}
	encrypted, err := enc.Encrypt([]byte(plaintext), aad)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	return RotationRow{
		Name:           name,
		CredentialType: credType,
		EncryptedValue: encrypted,
	}
}

// noopWriter is a RotationRowWriter that always succeeds.
func noopWriter(_ context.Context, _ RotatedRow) error {
	return nil
}

// collectingWriter returns a writer that records all rows written and a
// pointer to the slice for inspection.
func collectingWriter() (RotationRowWriter, *[]RotatedRow) {
	var collected []RotatedRow
	writer := func(_ context.Context, row RotatedRow) error {
		collected = append(collected, row)
		return nil
	}
	return writer, &collected
}

// failingWriter returns a writer that returns an error for specific
// credential names.
func failingWriter(failNames map[string]error) RotationRowWriter {
	return func(_ context.Context, row RotatedRow) error {
		if err, ok := failNames[row.Name]; ok {
			return err
		}
		return nil
	}
}

// ---------------------------------------------------------------------------
// RotateCredentialRow — single row rotation tests
// ---------------------------------------------------------------------------

func TestRotateCredentialRow_AlreadyRotated(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	// Row is encrypted with the NEW key — already rotated.
	row := encryptRow(t, newEnc, "cred-1", CredentialTypeGeneric, "secret-value")

	rotated, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err != nil {
		t.Fatalf("RotateCredentialRow failed: %v", err)
	}
	if rotated.WasReEncrypted {
		t.Error("WasReEncrypted should be false for already-rotated row")
	}
	if rotated.Name != "cred-1" {
		t.Errorf("Name = %q, want %q", rotated.Name, "cred-1")
	}
	if rotated.NewEncryptedValue != "" {
		t.Error("NewEncryptedValue should be empty when not re-encrypted")
	}
}

func TestRotateCredentialRow_NeedsReEncryption(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	// Row is encrypted with the PREVIOUS key — needs re-encryption.
	row := encryptRow(t, prevEnc, "cred-old", CredentialTypeGeneric, "old-secret")

	rotated, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err != nil {
		t.Fatalf("RotateCredentialRow failed: %v", err)
	}
	if !rotated.WasReEncrypted {
		t.Error("WasReEncrypted should be true")
	}
	if rotated.Name != "cred-old" {
		t.Errorf("Name = %q, want %q", rotated.Name, "cred-old")
	}
	if rotated.NewEncryptedValue == "" {
		t.Fatal("NewEncryptedValue should not be empty")
	}

	// Verify the new ciphertext can be decrypted with the new key.
	aad, _ := BuildAAD(CredentialTypeGeneric, "cred-old")
	plaintext, err := newEnc.Decrypt(rotated.NewEncryptedValue, aad)
	if err != nil {
		t.Fatalf("failed to decrypt re-encrypted value: %v", err)
	}
	defer ZeroBytes(plaintext)

	if string(plaintext) != "old-secret" {
		t.Errorf("decrypted value = %q, want %q", string(plaintext), "old-secret")
	}
}

func TestRotateCredentialRow_BothKeysFail(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)
	thirdEnc := rotationEncryptor(t, 3) // neither new nor previous

	// Row encrypted with a third, unknown key.
	row := encryptRow(t, thirdEnc, "orphan", CredentialTypeGeneric, "orphan-secret")

	_, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err == nil {
		t.Fatal("expected error when both keys fail")
	}
	if !errors.Is(err, ErrRotationDecryptFailed) {
		t.Errorf("expected ErrRotationDecryptFailed, got: %v", err)
	}
	if !strings.Contains(err.Error(), "orphan") {
		t.Errorf("error should mention credential name, got: %v", err)
	}
}

func TestRotateCredentialRow_NilNewEncryptor(t *testing.T) {
	prevEnc := rotationEncryptor(t, 2)
	row := encryptRow(t, prevEnc, "cred", CredentialTypeGeneric, "val")

	_, err := RotateCredentialRow(row, nil, prevEnc)
	if !errors.Is(err, ErrMasterKeyRequired) {
		t.Fatalf("expected ErrMasterKeyRequired, got: %v", err)
	}
}

func TestRotateCredentialRow_NilPrevEncryptor(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	row := encryptRow(t, newEnc, "cred", CredentialTypeGeneric, "val")

	_, err := RotateCredentialRow(row, newEnc, nil)
	if !errors.Is(err, ErrPreviousKeyRequired) {
		t.Fatalf("expected ErrPreviousKeyRequired, got: %v", err)
	}
}

func TestRotateCredentialRow_PlaintextZeroed(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	// Encrypt with previous key so re-encryption occurs and plaintext is
	// created in memory. We can't directly inspect the internal plaintext
	// variable, but we verify the function completes without leaking data
	// by checking the output is correct.
	row := encryptRow(t, prevEnc, "zero-test", CredentialTypeGeneric, "must-be-zeroed")

	rotated, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err != nil {
		t.Fatalf("RotateCredentialRow failed: %v", err)
	}
	if !rotated.WasReEncrypted {
		t.Error("should have been re-encrypted")
	}

	// Decrypt with new key to confirm the value is correct.
	aad, _ := BuildAAD(CredentialTypeGeneric, "zero-test")
	pt, err := newEnc.Decrypt(rotated.NewEncryptedValue, aad)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	defer ZeroBytes(pt)
	if string(pt) != "must-be-zeroed" {
		t.Errorf("value = %q", string(pt))
	}
}

func TestRotateCredentialRow_PreservesCredentialType(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	types := []string{
		CredentialTypeGeneric,
		CredentialTypeWebhookURL,
		CredentialTypeLDAPBindPassword,
		CredentialTypeSMTPPassword,
	}

	for _, ct := range types {
		t.Run(ct, func(t *testing.T) {
			row := encryptRow(t, prevEnc, "type-test-"+ct, ct, "value-for-"+ct)

			rotated, err := RotateCredentialRow(row, newEnc, prevEnc)
			if err != nil {
				t.Fatalf("RotateCredentialRow failed: %v", err)
			}
			if !rotated.WasReEncrypted {
				t.Error("should have been re-encrypted")
			}

			// Decrypt and verify AAD binding is correct.
			aad, _ := BuildAAD(ct, "type-test-"+ct)
			pt, err := newEnc.Decrypt(rotated.NewEncryptedValue, aad)
			if err != nil {
				t.Fatalf("decrypt with correct AAD failed: %v", err)
			}
			ZeroBytes(pt)
		})
	}
}

func TestRotateCredentialRow_AADBindingPreserved(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	row := encryptRow(t, prevEnc, "aad-test", CredentialTypeGeneric, "aad-value")

	rotated, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err != nil {
		t.Fatalf("RotateCredentialRow failed: %v", err)
	}

	// Trying to decrypt with wrong AAD should fail.
	wrongAAD, _ := BuildAAD(CredentialTypeWebhookURL, "aad-test")
	_, err = newEnc.Decrypt(rotated.NewEncryptedValue, wrongAAD)
	if err == nil {
		t.Error("decryption with wrong AAD should fail (binding must be preserved)")
	}

	// Trying to decrypt with wrong name should fail.
	wrongNameAAD, _ := BuildAAD(CredentialTypeGeneric, "wrong-name")
	_, err = newEnc.Decrypt(rotated.NewEncryptedValue, wrongNameAAD)
	if err == nil {
		t.Error("decryption with wrong name should fail")
	}
}

func TestRotateCredentialRow_NewCiphertextDiffersFromOld(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	row := encryptRow(t, prevEnc, "diff-test", CredentialTypeGeneric, "same-plaintext")

	rotated, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err != nil {
		t.Fatalf("RotateCredentialRow failed: %v", err)
	}

	if rotated.NewEncryptedValue == row.EncryptedValue {
		t.Error("new ciphertext should differ from old ciphertext")
	}
}

func TestRotateCredentialRow_EmptyEncryptedValue(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	row := RotationRow{
		Name:           "empty-cipher",
		CredentialType: CredentialTypeGeneric,
		EncryptedValue: "",
	}

	_, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err == nil {
		t.Fatal("expected error for empty encrypted value")
	}
}

func TestRotateCredentialRow_GarbageEncryptedValue(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	row := RotationRow{
		Name:           "garbage",
		CredentialType: CredentialTypeGeneric,
		EncryptedValue: "not-valid-ciphertext",
	}

	_, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err == nil {
		t.Fatal("expected error for garbage ciphertext")
	}
	if !errors.Is(err, ErrRotationDecryptFailed) {
		t.Errorf("expected ErrRotationDecryptFailed, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RotateMasterKey — batch rotation tests
// ---------------------------------------------------------------------------

func TestRotateMasterKey_AllNeedReEncryption(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	writer, collected := collectingWriter()

	rows := []RotationRow{
		encryptRow(t, prevEnc, "cred-a", CredentialTypeGeneric, "secret-a"),
		encryptRow(t, prevEnc, "cred-b", CredentialTypeLDAPBindPassword, "secret-b"),
		encryptRow(t, prevEnc, "cred-c", CredentialTypeSMTPPassword, "secret-c"),
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.TotalCredentials != 3 {
		t.Errorf("TotalCredentials = %d, want 3", result.TotalCredentials)
	}
	if result.ReEncrypted != 3 {
		t.Errorf("ReEncrypted = %d, want 3", result.ReEncrypted)
	}
	if result.AlreadyRotated != 0 {
		t.Errorf("AlreadyRotated = %d, want 0", result.AlreadyRotated)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v, want empty", result.Errors)
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}

	if len(*collected) != 3 {
		t.Fatalf("writer called %d times, want 3", len(*collected))
	}

	// Verify each written row can be decrypted with the new key.
	for _, written := range *collected {
		if !written.WasReEncrypted {
			t.Errorf("written row %q should be marked as re-encrypted", written.Name)
		}
		aad, _ := BuildAAD(getCredType(t, rows, written.Name), written.Name)
		pt, err := newEnc.Decrypt(written.NewEncryptedValue, aad)
		if err != nil {
			t.Fatalf("failed to decrypt re-encrypted row %q: %v", written.Name, err)
		}
		ZeroBytes(pt)
	}
}

func TestRotateMasterKey_AllAlreadyRotated(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	writer, collected := collectingWriter()

	// All rows encrypted with the NEW key — nothing to do.
	rows := []RotationRow{
		encryptRow(t, newEnc, "new-a", CredentialTypeGeneric, "val-a"),
		encryptRow(t, newEnc, "new-b", CredentialTypeGeneric, "val-b"),
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.TotalCredentials != 2 {
		t.Errorf("TotalCredentials = %d, want 2", result.TotalCredentials)
	}
	if result.AlreadyRotated != 2 {
		t.Errorf("AlreadyRotated = %d, want 2", result.AlreadyRotated)
	}
	if result.ReEncrypted != 0 {
		t.Errorf("ReEncrypted = %d, want 0", result.ReEncrypted)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if len(*collected) != 0 {
		t.Errorf("writer should not have been called, got %d calls", len(*collected))
	}
}

func TestRotateMasterKey_MixedState(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	writer, collected := collectingWriter()

	rows := []RotationRow{
		encryptRow(t, newEnc, "already-new", CredentialTypeGeneric, "val-new"),
		encryptRow(t, prevEnc, "needs-rotate", CredentialTypeGeneric, "val-old"),
		encryptRow(t, newEnc, "also-new", CredentialTypeGeneric, "val-new-2"),
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.AlreadyRotated != 2 {
		t.Errorf("AlreadyRotated = %d, want 2", result.AlreadyRotated)
	}
	if result.ReEncrypted != 1 {
		t.Errorf("ReEncrypted = %d, want 1", result.ReEncrypted)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if len(*collected) != 1 {
		t.Fatalf("writer should be called once, got %d", len(*collected))
	}
	if (*collected)[0].Name != "needs-rotate" {
		t.Errorf("written row = %q, want %q", (*collected)[0].Name, "needs-rotate")
	}
}

func TestRotateMasterKey_SomeDecryptionFailures(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	unknownEnc := rotationEncryptor(t, 99) // neither new nor previous

	rows := []RotationRow{
		encryptRow(t, prevEnc, "good-1", CredentialTypeGeneric, "val-1"),
		encryptRow(t, unknownEnc, "bad-1", CredentialTypeGeneric, "val-bad"),
		encryptRow(t, prevEnc, "good-2", CredentialTypeGeneric, "val-2"),
		encryptRow(t, unknownEnc, "bad-2", CredentialTypeGeneric, "val-bad-2"),
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, noopWriter)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.ReEncrypted != 2 {
		t.Errorf("ReEncrypted = %d, want 2", result.ReEncrypted)
	}
	if result.Failed != 2 {
		t.Errorf("Failed = %d, want 2", result.Failed)
	}
	if len(result.Errors) != 2 {
		t.Fatalf("Errors count = %d, want 2", len(result.Errors))
	}
	if _, ok := result.Errors["bad-1"]; !ok {
		t.Error("Errors should contain 'bad-1'")
	}
	if _, ok := result.Errors["bad-2"]; !ok {
		t.Error("Errors should contain 'bad-2'")
	}
}

func TestRotateMasterKey_WriterErrors(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	rows := []RotationRow{
		encryptRow(t, prevEnc, "ok-cred", CredentialTypeGeneric, "val-ok"),
		encryptRow(t, prevEnc, "fail-write", CredentialTypeGeneric, "val-fail"),
		encryptRow(t, prevEnc, "ok-cred-2", CredentialTypeGeneric, "val-ok-2"),
	}

	writer := failingWriter(map[string]error{
		"fail-write": fmt.Errorf("disk full"),
	})

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.ReEncrypted != 2 {
		t.Errorf("ReEncrypted = %d, want 2", result.ReEncrypted)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors count = %d, want 1", len(result.Errors))
	}
	failErr, ok := result.Errors["fail-write"]
	if !ok {
		t.Fatal("Errors should contain 'fail-write'")
	}
	if !strings.Contains(failErr.Error(), "disk full") {
		t.Errorf("error should mention underlying cause, got: %v", failErr)
	}
}

func TestRotateMasterKey_AllWriterErrors(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	rows := []RotationRow{
		encryptRow(t, prevEnc, "w1", CredentialTypeGeneric, "v1"),
		encryptRow(t, prevEnc, "w2", CredentialTypeGeneric, "v2"),
	}

	writer := func(_ context.Context, _ RotatedRow) error {
		return fmt.Errorf("database unavailable")
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.ReEncrypted != 0 {
		t.Errorf("ReEncrypted = %d, want 0", result.ReEncrypted)
	}
	if result.Failed != 2 {
		t.Errorf("Failed = %d, want 2", result.Failed)
	}
}

func TestRotateMasterKey_EmptyRowList(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	result, err := RotateMasterKey(context.Background(), nil, newEnc, prevEnc, noopWriter)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.TotalCredentials != 0 {
		t.Errorf("TotalCredentials = %d, want 0", result.TotalCredentials)
	}
	if result.ReEncrypted != 0 {
		t.Errorf("ReEncrypted = %d, want 0", result.ReEncrypted)
	}
	if result.AlreadyRotated != 0 {
		t.Errorf("AlreadyRotated = %d, want 0", result.AlreadyRotated)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if result.Duration < 0 {
		t.Error("Duration should be non-negative")
	}
}

func TestRotateMasterKey_NilNewEncryptor(t *testing.T) {
	prevEnc := rotationEncryptor(t, 20)
	_, err := RotateMasterKey(context.Background(), nil, nil, prevEnc, noopWriter)
	if !errors.Is(err, ErrMasterKeyRequired) {
		t.Fatalf("expected ErrMasterKeyRequired, got: %v", err)
	}
}

func TestRotateMasterKey_NilPrevEncryptor(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	_, err := RotateMasterKey(context.Background(), nil, newEnc, nil, noopWriter)
	if !errors.Is(err, ErrPreviousKeyRequired) {
		t.Fatalf("expected ErrPreviousKeyRequired, got: %v", err)
	}
}

func TestRotateMasterKey_NilWriter(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	_, err := RotateMasterKey(context.Background(), nil, newEnc, prevEnc, nil)
	if err == nil {
		t.Fatal("expected error for nil writer")
	}
	if !strings.Contains(err.Error(), "writer") {
		t.Errorf("error should mention writer, got: %v", err)
	}
}

func TestRotateMasterKey_ContextCancellation(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	// Create 10 rows that need re-encryption.
	rows := make([]RotationRow, 10)
	for i := range rows {
		rows[i] = encryptRow(t, prevEnc, fmt.Sprintf("ctx-cred-%d", i), CredentialTypeGeneric, fmt.Sprintf("val-%d", i))
	}

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	writer := func(_ context.Context, _ RotatedRow) error {
		callCount++
		if callCount >= 3 {
			cancel() // Cancel after 3 successful writes.
		}
		return nil
	}

	result, err := RotateMasterKey(ctx, rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey should not return error on cancellation, got: %v", err)
	}

	// At least 3 should have been re-encrypted before cancellation kicked in.
	if result.ReEncrypted < 3 {
		t.Errorf("ReEncrypted = %d, expected at least 3", result.ReEncrypted)
	}
	// Total of re-encrypted + failed should equal total.
	total := result.AlreadyRotated + result.ReEncrypted + result.Failed
	if total != result.TotalCredentials {
		t.Errorf("AlreadyRotated(%d) + ReEncrypted(%d) + Failed(%d) = %d, want TotalCredentials(%d)",
			result.AlreadyRotated, result.ReEncrypted, result.Failed, total, result.TotalCredentials)
	}
	// There should be a context error recorded.
	if _, ok := result.Errors["_context"]; !ok {
		t.Error("expected _context error entry on cancellation")
	}
}

// ---------------------------------------------------------------------------
// Crash recovery simulation
// ---------------------------------------------------------------------------

func TestRotateMasterKey_CrashRecovery_PartialRotation(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	// Simulate a crash mid-rotation: some rows are already re-encrypted
	// (from a previous partial run), others still use the old key.
	rows := []RotationRow{
		encryptRow(t, newEnc, "already-done-1", CredentialTypeGeneric, "val-1"),
		encryptRow(t, newEnc, "already-done-2", CredentialTypeGeneric, "val-2"),
		encryptRow(t, prevEnc, "still-old-1", CredentialTypeGeneric, "val-3"),
		encryptRow(t, prevEnc, "still-old-2", CredentialTypeGeneric, "val-4"),
	}

	writer, collected := collectingWriter()

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.AlreadyRotated != 2 {
		t.Errorf("AlreadyRotated = %d, want 2", result.AlreadyRotated)
	}
	if result.ReEncrypted != 2 {
		t.Errorf("ReEncrypted = %d, want 2", result.ReEncrypted)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}

	// Only the old-key rows should have been written.
	if len(*collected) != 2 {
		t.Fatalf("writer called %d times, want 2", len(*collected))
	}
	writtenNames := make(map[string]bool)
	for _, w := range *collected {
		writtenNames[w.Name] = true
	}
	if !writtenNames["still-old-1"] || !writtenNames["still-old-2"] {
		t.Errorf("expected still-old-1 and still-old-2 to be written, got: %v", writtenNames)
	}
}

func TestRotateMasterKey_IdempotentRerun(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	originalRows := []RotationRow{
		encryptRow(t, prevEnc, "idem-1", CredentialTypeGeneric, "val-1"),
		encryptRow(t, prevEnc, "idem-2", CredentialTypeGeneric, "val-2"),
	}

	// First run: re-encrypt everything.
	writer1, collected1 := collectingWriter()
	result1, err := RotateMasterKey(context.Background(), originalRows, newEnc, prevEnc, writer1)
	if err != nil {
		t.Fatalf("first rotation failed: %v", err)
	}
	if result1.ReEncrypted != 2 {
		t.Fatalf("first run: ReEncrypted = %d, want 2", result1.ReEncrypted)
	}

	// Build rows as if the database was updated with the new ciphertexts.
	updatedRows := make([]RotationRow, len(*collected1))
	for i, w := range *collected1 {
		updatedRows[i] = RotationRow{
			Name:           w.Name,
			CredentialType: CredentialTypeGeneric,
			EncryptedValue: w.NewEncryptedValue,
		}
	}

	// Second run: everything should be "already rotated".
	writer2, collected2 := collectingWriter()
	result2, err := RotateMasterKey(context.Background(), updatedRows, newEnc, prevEnc, writer2)
	if err != nil {
		t.Fatalf("second rotation failed: %v", err)
	}
	if result2.AlreadyRotated != 2 {
		t.Errorf("second run: AlreadyRotated = %d, want 2", result2.AlreadyRotated)
	}
	if result2.ReEncrypted != 0 {
		t.Errorf("second run: ReEncrypted = %d, want 0", result2.ReEncrypted)
	}
	if len(*collected2) != 0 {
		t.Errorf("second run: writer should not be called, got %d calls", len(*collected2))
	}
}

// ---------------------------------------------------------------------------
// RotateMasterKey — large batch
// ---------------------------------------------------------------------------

func TestRotateMasterKey_LargeBatch(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	const count = 100
	rows := make([]RotationRow, count)
	for i := range rows {
		rows[i] = encryptRow(t, prevEnc, fmt.Sprintf("large-%03d", i), CredentialTypeGeneric, fmt.Sprintf("value-%d", i))
	}

	writer, collected := collectingWriter()
	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.ReEncrypted != count {
		t.Errorf("ReEncrypted = %d, want %d", result.ReEncrypted, count)
	}
	if len(*collected) != count {
		t.Errorf("writer called %d times, want %d", len(*collected), count)
	}
}

// ---------------------------------------------------------------------------
// RotateMasterKey — result accounting is always consistent
// ---------------------------------------------------------------------------

func TestRotateMasterKey_ResultAccounting(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	unknownEnc := rotationEncryptor(t, 99)

	rows := []RotationRow{
		encryptRow(t, newEnc, "acc-new", CredentialTypeGeneric, "v"),
		encryptRow(t, prevEnc, "acc-old", CredentialTypeGeneric, "v"),
		encryptRow(t, unknownEnc, "acc-bad", CredentialTypeGeneric, "v"),
	}

	writer := failingWriter(map[string]error{
		// No writer failures — let the unknown-key failure come from decrypt.
	})

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	total := result.AlreadyRotated + result.ReEncrypted + result.Failed
	if total != result.TotalCredentials {
		t.Errorf("AlreadyRotated(%d) + ReEncrypted(%d) + Failed(%d) = %d, want TotalCredentials(%d)",
			result.AlreadyRotated, result.ReEncrypted, result.Failed, total, result.TotalCredentials)
	}
}

// ---------------------------------------------------------------------------
// NeedsRotation
// ---------------------------------------------------------------------------

func TestNeedsRotation_BothKeysSet(t *testing.T) {
	env := fakeEnv(map[string]string{
		"CMM_CREDENTIAL_ENCRYPTION_KEY":          "new-key-base64",
		"CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS": "old-key-base64",
	})
	if !NeedsRotation(env) {
		t.Error("should need rotation when previous key is set")
	}
}

func TestNeedsRotation_OnlyCurrentKey(t *testing.T) {
	env := fakeEnv(map[string]string{
		"CMM_CREDENTIAL_ENCRYPTION_KEY": "new-key-base64",
	})
	if NeedsRotation(env) {
		t.Error("should not need rotation when only current key is set")
	}
}

func TestNeedsRotation_PreviousKeyEmpty(t *testing.T) {
	env := fakeEnv(map[string]string{
		"CMM_CREDENTIAL_ENCRYPTION_KEY":          "new-key-base64",
		"CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS": "",
	})
	if NeedsRotation(env) {
		t.Error("should not need rotation when previous key is empty string")
	}
}

func TestNeedsRotation_NoKeysSet(t *testing.T) {
	env := emptyEnv()
	if NeedsRotation(env) {
		t.Error("should not need rotation when no keys are set")
	}
}

func TestNeedsRotation_NilEnvFunc(t *testing.T) {
	if NeedsRotation(nil) {
		t.Error("should not need rotation with nil env func")
	}
}

func TestNeedsRotation_OnlyPreviousKeySet(t *testing.T) {
	env := fakeEnv(map[string]string{
		"CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS": "old-key-base64",
	})
	// The function only checks for the previous key, not the current key.
	if !NeedsRotation(env) {
		t.Error("should need rotation when previous key is set (even without current)")
	}
}

// ---------------------------------------------------------------------------
// Sentinel error identity tests
// ---------------------------------------------------------------------------

func TestRotationSentinelErrors_AreDistinct(t *testing.T) {
	sentinels := []error{
		ErrPreviousKeyRequired,
		ErrRotationDecryptFailed,
		ErrMasterKeyRequired,
	}

	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			if errors.Is(sentinels[i], sentinels[j]) {
				t.Errorf("sentinel errors should be distinct: %v == %v", sentinels[i], sentinels[j])
			}
		}
	}
}

func TestRotationSentinelErrors_HaveSecretsPrefix(t *testing.T) {
	sentinels := map[string]error{
		"ErrPreviousKeyRequired":   ErrPreviousKeyRequired,
		"ErrRotationDecryptFailed": ErrRotationDecryptFailed,
	}
	for name, err := range sentinels {
		if !strings.HasPrefix(err.Error(), "secrets:") {
			t.Errorf("%s should start with 'secrets:', got %q", name, err.Error())
		}
	}
}

// ---------------------------------------------------------------------------
// RotationResult struct tests
// ---------------------------------------------------------------------------

func TestRotationResult_EmptyErrorsMap(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	result, err := RotateMasterKey(context.Background(), nil, newEnc, prevEnc, noopWriter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Errors == nil {
		t.Error("Errors map should be non-nil (empty, not nil)")
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors should be empty, got %d entries", len(result.Errors))
	}
}

func TestRotationResult_Duration_IsPositive(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	rows := []RotationRow{
		encryptRow(t, prevEnc, "dur-test", CredentialTypeGeneric, "value"),
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, noopWriter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive when rows are processed")
	}
}

// ---------------------------------------------------------------------------
// RotationRow / RotatedRow struct tests
// ---------------------------------------------------------------------------

func TestRotationRow_Fields(t *testing.T) {
	row := RotationRow{
		Name:           "test",
		CredentialType: CredentialTypeGeneric,
		EncryptedValue: "abc:def",
	}
	if row.Name != "test" || row.CredentialType != CredentialTypeGeneric || row.EncryptedValue != "abc:def" {
		t.Error("RotationRow fields should be populated correctly")
	}
}

func TestRotatedRow_Fields(t *testing.T) {
	row := RotatedRow{
		Name:              "test",
		NewEncryptedValue: "newcipher",
		WasReEncrypted:    true,
	}
	if row.Name != "test" || row.NewEncryptedValue != "newcipher" || !row.WasReEncrypted {
		t.Error("RotatedRow fields should be populated correctly")
	}
}

// ---------------------------------------------------------------------------
// RotateCredentialRow — additional edge case tests
// ---------------------------------------------------------------------------

func TestRotateCredentialRow_ChefClientKeyType(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	// Use CredentialTypeChefClientKey which is not covered in
	// PreservesCredentialType (that test only covers generic, webhook, ldap,
	// smtp).
	row := encryptRow(t, prevEnc, "chef-key-1", CredentialTypeChefClientKey, "-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----")

	rotated, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err != nil {
		t.Fatalf("RotateCredentialRow failed: %v", err)
	}
	if !rotated.WasReEncrypted {
		t.Error("should have been re-encrypted")
	}

	aad, _ := BuildAAD(CredentialTypeChefClientKey, "chef-key-1")
	pt, err := newEnc.Decrypt(rotated.NewEncryptedValue, aad)
	if err != nil {
		t.Fatalf("decrypt with correct AAD failed: %v", err)
	}
	if !strings.Contains(string(pt), "BEGIN RSA PRIVATE KEY") {
		t.Errorf("plaintext should contain PEM header, got %q", string(pt))
	}
	ZeroBytes(pt)
}

func TestRotateCredentialRow_SameKeyForBothEncryptors(t *testing.T) {
	// When the same master key is used for both new and previous encryptors,
	// every row should be detected as "already rotated" on the first decrypt
	// attempt (new key succeeds).
	key := rotationKey(t, 42)
	enc1, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor failed: %v", err)
	}
	t.Cleanup(func() { enc1.Close() })

	enc2, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor failed: %v", err)
	}
	t.Cleanup(func() { enc2.Close() })

	row := encryptRow(t, enc1, "same-key-cred", CredentialTypeGeneric, "value")

	rotated, err := RotateCredentialRow(row, enc1, enc2)
	if err != nil {
		t.Fatalf("RotateCredentialRow failed: %v", err)
	}
	if rotated.WasReEncrypted {
		t.Error("WasReEncrypted should be false when both encryptors use the same key")
	}
}

func TestRotateCredentialRow_EmptyName(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	row := RotationRow{
		Name:           "",
		CredentialType: CredentialTypeGeneric,
		EncryptedValue: "aabbcc:ddeeff",
	}

	_, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err == nil {
		t.Fatal("expected error for empty credential name (AAD construction should fail)")
	}
	if !errors.Is(err, ErrAADRequired) {
		// The error is wrapped, so check for the underlying cause.
		if !strings.Contains(err.Error(), "associated data") {
			t.Errorf("error should relate to AAD, got: %v", err)
		}
	}
}

func TestRotateCredentialRow_EmptyCredentialType(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	row := RotationRow{
		Name:           "some-cred",
		CredentialType: "",
		EncryptedValue: "aabbcc:ddeeff",
	}

	_, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err == nil {
		t.Fatal("expected error for empty credential type (AAD construction should fail)")
	}
}

func TestRotateCredentialRow_TruncatedCiphertext(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	// Valid hex format but the ciphertext portion is too short to be a valid
	// GCM ciphertext (needs at least a 16-byte tag).
	row := RotationRow{
		Name:           "truncated",
		CredentialType: CredentialTypeGeneric,
		EncryptedValue: "aabbccddeeff00112233445566:aabb",
	}

	_, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err == nil {
		t.Fatal("expected error for truncated ciphertext")
	}
	if !errors.Is(err, ErrRotationDecryptFailed) {
		t.Errorf("expected ErrRotationDecryptFailed, got: %v", err)
	}
}

func TestRotateCredentialRow_LargePlaintext(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	// Simulate a large credential value (e.g. a multi-line PEM key).
	largePlaintext := strings.Repeat("A", 8192)
	row := encryptRow(t, prevEnc, "large-val", CredentialTypeGeneric, largePlaintext)

	rotated, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err != nil {
		t.Fatalf("RotateCredentialRow failed: %v", err)
	}
	if !rotated.WasReEncrypted {
		t.Error("should have been re-encrypted")
	}

	aad, _ := BuildAAD(CredentialTypeGeneric, "large-val")
	pt, err := newEnc.Decrypt(rotated.NewEncryptedValue, aad)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if string(pt) != largePlaintext {
		t.Errorf("plaintext length = %d, want %d", len(pt), len(largePlaintext))
	}
	ZeroBytes(pt)
}

func TestRotateCredentialRow_ErrorWrapping_DecryptFailed(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)
	thirdEnc := rotationEncryptor(t, 3)

	row := encryptRow(t, thirdEnc, "wrap-test", CredentialTypeGeneric, "val")

	_, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err == nil {
		t.Fatal("expected error")
	}
	// Verify errors.Is works through the wrapping.
	if !errors.Is(err, ErrRotationDecryptFailed) {
		t.Errorf("errors.Is should match ErrRotationDecryptFailed, got: %v", err)
	}
	// Verify the credential name appears in the error message.
	if !strings.Contains(err.Error(), "wrap-test") {
		t.Errorf("error should contain credential name, got: %v", err)
	}
	// Verify both key failures are mentioned.
	if !strings.Contains(err.Error(), "new key") {
		t.Errorf("error should mention 'new key', got: %v", err)
	}
	if !strings.Contains(err.Error(), "previous key") {
		t.Errorf("error should mention 'previous key', got: %v", err)
	}
}

func TestRotateCredentialRow_ErrorWrapping_AADFailure(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	row := RotationRow{
		Name:           "",
		CredentialType: "",
		EncryptedValue: "aa:bb",
	}

	_, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err == nil {
		t.Fatal("expected error for empty AAD fields")
	}
	// The error wraps the rotation failure context.
	if !strings.Contains(err.Error(), "rotation failed") {
		t.Errorf("error should mention rotation failure, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RotateMasterKey — additional batch tests
// ---------------------------------------------------------------------------

func TestRotateMasterKey_SingleRow(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	writer, collected := collectingWriter()

	rows := []RotationRow{
		encryptRow(t, prevEnc, "only-one", CredentialTypeGeneric, "single-value"),
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.TotalCredentials != 1 {
		t.Errorf("TotalCredentials = %d, want 1", result.TotalCredentials)
	}
	if result.ReEncrypted != 1 {
		t.Errorf("ReEncrypted = %d, want 1", result.ReEncrypted)
	}
	if len(*collected) != 1 {
		t.Fatalf("writer called %d times, want 1", len(*collected))
	}

	// Verify the written value is decryptable.
	aad, _ := BuildAAD(CredentialTypeGeneric, "only-one")
	pt, err := newEnc.Decrypt((*collected)[0].NewEncryptedValue, aad)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if string(pt) != "single-value" {
		t.Errorf("plaintext = %q, want %q", string(pt), "single-value")
	}
	ZeroBytes(pt)
}

func TestRotateMasterKey_MixedCredentialTypes_ValuesPreserved(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	writer, collected := collectingWriter()

	// Use every credential type to verify rotation handles them all.
	rows := []RotationRow{
		encryptRow(t, prevEnc, "key-1", CredentialTypeChefClientKey, "-----BEGIN RSA PRIVATE KEY-----\nfake-key\n-----END RSA PRIVATE KEY-----"),
		encryptRow(t, prevEnc, "ldap-1", CredentialTypeLDAPBindPassword, "ldap-secret"),
		encryptRow(t, prevEnc, "smtp-1", CredentialTypeSMTPPassword, "smtp-secret"),
		encryptRow(t, prevEnc, "hook-1", CredentialTypeWebhookURL, "https://example.com/hook"),
		encryptRow(t, prevEnc, "gen-1", CredentialTypeGeneric, "generic-secret"),
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.ReEncrypted != 5 {
		t.Errorf("ReEncrypted = %d, want 5", result.ReEncrypted)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}

	// Build expected plaintext values keyed by name.
	expected := map[string]string{
		"key-1":  "-----BEGIN RSA PRIVATE KEY-----\nfake-key\n-----END RSA PRIVATE KEY-----",
		"ldap-1": "ldap-secret",
		"smtp-1": "smtp-secret",
		"hook-1": "https://example.com/hook",
		"gen-1":  "generic-secret",
	}

	for _, w := range *collected {
		credType := getCredType(t, rows, w.Name)
		aad, _ := BuildAAD(credType, w.Name)
		pt, err := newEnc.Decrypt(w.NewEncryptedValue, aad)
		if err != nil {
			t.Fatalf("decrypt failed for %q: %v", w.Name, err)
		}
		want, ok := expected[w.Name]
		if !ok {
			t.Fatalf("unexpected written row: %q", w.Name)
		}
		if string(pt) != want {
			t.Errorf("plaintext for %q = %q, want %q", w.Name, string(pt), want)
		}
		ZeroBytes(pt)
	}
}

func TestRotateMasterKey_ContextAlreadyCancelled(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	rows := []RotationRow{
		encryptRow(t, prevEnc, "never-processed", CredentialTypeGeneric, "val"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before rotation starts.

	result, err := RotateMasterKey(ctx, rows, newEnc, prevEnc, noopWriter)
	if err != nil {
		t.Fatalf("RotateMasterKey should not return error, got: %v", err)
	}

	// No rows should have been re-encrypted because context was already done.
	if result.ReEncrypted != 0 {
		t.Errorf("ReEncrypted = %d, want 0", result.ReEncrypted)
	}
	// The remaining rows should be counted as failed.
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if _, ok := result.Errors["_context"]; !ok {
		t.Error("expected _context error entry")
	}
	// Accounting should still be consistent.
	total := result.AlreadyRotated + result.ReEncrypted + result.Failed
	if total != result.TotalCredentials {
		t.Errorf("accounting mismatch: %d + %d + %d = %d, want %d",
			result.AlreadyRotated, result.ReEncrypted, result.Failed, total, result.TotalCredentials)
	}
}

func TestRotateMasterKey_ProcessingOrder(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	rows := []RotationRow{
		encryptRow(t, prevEnc, "order-a", CredentialTypeGeneric, "val-a"),
		encryptRow(t, prevEnc, "order-b", CredentialTypeGeneric, "val-b"),
		encryptRow(t, prevEnc, "order-c", CredentialTypeGeneric, "val-c"),
		encryptRow(t, prevEnc, "order-d", CredentialTypeGeneric, "val-d"),
	}

	var order []string
	writer := func(_ context.Context, row RotatedRow) error {
		order = append(order, row.Name)
		return nil
	}

	_, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	expectedOrder := []string{"order-a", "order-b", "order-c", "order-d"}
	if len(order) != len(expectedOrder) {
		t.Fatalf("writer called %d times, want %d", len(order), len(expectedOrder))
	}
	for i, name := range expectedOrder {
		if order[i] != name {
			t.Errorf("order[%d] = %q, want %q", i, order[i], name)
		}
	}
}

func TestRotateMasterKey_AllDecryptionFailures(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	unknownEnc := rotationEncryptor(t, 99)

	rows := []RotationRow{
		encryptRow(t, unknownEnc, "bad-a", CredentialTypeGeneric, "val-a"),
		encryptRow(t, unknownEnc, "bad-b", CredentialTypeGeneric, "val-b"),
		encryptRow(t, unknownEnc, "bad-c", CredentialTypeGeneric, "val-c"),
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, noopWriter)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.TotalCredentials != 3 {
		t.Errorf("TotalCredentials = %d, want 3", result.TotalCredentials)
	}
	if result.Failed != 3 {
		t.Errorf("Failed = %d, want 3", result.Failed)
	}
	if result.ReEncrypted != 0 {
		t.Errorf("ReEncrypted = %d, want 0", result.ReEncrypted)
	}
	if result.AlreadyRotated != 0 {
		t.Errorf("AlreadyRotated = %d, want 0", result.AlreadyRotated)
	}
	if len(result.Errors) != 3 {
		t.Errorf("Errors count = %d, want 3", len(result.Errors))
	}
	for _, name := range []string{"bad-a", "bad-b", "bad-c"} {
		if _, ok := result.Errors[name]; !ok {
			t.Errorf("Errors should contain %q", name)
		}
	}
}

func TestRotateMasterKey_SameKeyNewAndPrevious(t *testing.T) {
	key := rotationKey(t, 42)
	enc1, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor failed: %v", err)
	}
	t.Cleanup(func() { enc1.Close() })

	enc2, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor failed: %v", err)
	}
	t.Cleanup(func() { enc2.Close() })

	rows := []RotationRow{
		encryptRow(t, enc1, "same-a", CredentialTypeGeneric, "val-a"),
		encryptRow(t, enc1, "same-b", CredentialTypeGeneric, "val-b"),
	}

	writer, collected := collectingWriter()
	result, err := RotateMasterKey(context.Background(), rows, enc1, enc2, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	// All rows should be detected as already using the new key.
	if result.AlreadyRotated != 2 {
		t.Errorf("AlreadyRotated = %d, want 2", result.AlreadyRotated)
	}
	if result.ReEncrypted != 0 {
		t.Errorf("ReEncrypted = %d, want 0", result.ReEncrypted)
	}
	if len(*collected) != 0 {
		t.Errorf("writer should not have been called, got %d calls", len(*collected))
	}
}

func TestRotateMasterKey_WriterErrorDoesNotAbortRemaining(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	rows := []RotationRow{
		encryptRow(t, prevEnc, "w-a", CredentialTypeGeneric, "v-a"),
		encryptRow(t, prevEnc, "w-b", CredentialTypeGeneric, "v-b"),
		encryptRow(t, prevEnc, "w-c", CredentialTypeGeneric, "v-c"),
		encryptRow(t, prevEnc, "w-d", CredentialTypeGeneric, "v-d"),
	}

	// Fail writes for the first and third rows.
	writer := failingWriter(map[string]error{
		"w-a": fmt.Errorf("write error a"),
		"w-c": fmt.Errorf("write error c"),
	})

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.ReEncrypted != 2 {
		t.Errorf("ReEncrypted = %d, want 2", result.ReEncrypted)
	}
	if result.Failed != 2 {
		t.Errorf("Failed = %d, want 2", result.Failed)
	}
	// Verify the correct rows failed.
	if _, ok := result.Errors["w-a"]; !ok {
		t.Error("Errors should contain 'w-a'")
	}
	if _, ok := result.Errors["w-c"]; !ok {
		t.Error("Errors should contain 'w-c'")
	}
	// Verify the successful rows are not in errors.
	if _, ok := result.Errors["w-b"]; ok {
		t.Error("Errors should not contain 'w-b'")
	}
	if _, ok := result.Errors["w-d"]; ok {
		t.Error("Errors should not contain 'w-d'")
	}
}

func TestRotateMasterKey_WriterErrorMessage(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	rows := []RotationRow{
		encryptRow(t, prevEnc, "err-msg-cred", CredentialTypeGeneric, "val"),
	}

	writer := failingWriter(map[string]error{
		"err-msg-cred": fmt.Errorf("underlying write failure"),
	})

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	credErr, ok := result.Errors["err-msg-cred"]
	if !ok {
		t.Fatal("expected error for err-msg-cred")
	}
	// The error should wrap the original cause and mention the credential name.
	if !strings.Contains(credErr.Error(), "err-msg-cred") {
		t.Errorf("error should mention credential name, got: %v", credErr)
	}
	if !strings.Contains(credErr.Error(), "underlying write failure") {
		t.Errorf("error should contain underlying cause, got: %v", credErr)
	}
}

func TestRotateMasterKey_MixedFailuresAndSuccesses_Accounting(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	unknownEnc := rotationEncryptor(t, 99)

	rows := []RotationRow{
		encryptRow(t, newEnc, "already-1", CredentialTypeGeneric, "v"),    // already rotated
		encryptRow(t, prevEnc, "old-1", CredentialTypeGeneric, "v"),       // needs re-encryption, writer succeeds
		encryptRow(t, unknownEnc, "orphan-1", CredentialTypeGeneric, "v"), // decrypt fails
		encryptRow(t, prevEnc, "old-2", CredentialTypeGeneric, "v"),       // needs re-encryption, writer fails
		encryptRow(t, newEnc, "already-2", CredentialTypeGeneric, "v"),    // already rotated
	}

	writer := failingWriter(map[string]error{
		"old-2": fmt.Errorf("simulated failure"),
	})

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.TotalCredentials != 5 {
		t.Errorf("TotalCredentials = %d, want 5", result.TotalCredentials)
	}
	if result.AlreadyRotated != 2 {
		t.Errorf("AlreadyRotated = %d, want 2", result.AlreadyRotated)
	}
	if result.ReEncrypted != 1 {
		t.Errorf("ReEncrypted = %d, want 1", result.ReEncrypted)
	}
	if result.Failed != 2 {
		t.Errorf("Failed = %d, want 2", result.Failed)
	}
	total := result.AlreadyRotated + result.ReEncrypted + result.Failed
	if total != result.TotalCredentials {
		t.Errorf("accounting mismatch: %d + %d + %d = %d, want %d",
			result.AlreadyRotated, result.ReEncrypted, result.Failed, total, result.TotalCredentials)
	}
	if len(result.Errors) != 2 {
		t.Errorf("Errors count = %d, want 2", len(result.Errors))
	}
}

func TestRotateMasterKey_EmptyRowSlice_NotNil(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	// Pass an empty (non-nil) slice rather than nil.
	rows := []RotationRow{}
	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, noopWriter)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}
	if result.TotalCredentials != 0 {
		t.Errorf("TotalCredentials = %d, want 0", result.TotalCredentials)
	}
	if result.Duration < 0 {
		t.Error("Duration should be non-negative")
	}
	if result.Errors == nil {
		t.Error("Errors map should be non-nil")
	}
}

func TestRotateMasterKey_ContextDeadlineExceeded(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	rows := make([]RotationRow, 5)
	for i := range rows {
		rows[i] = encryptRow(t, prevEnc, fmt.Sprintf("deadline-%d", i), CredentialTypeGeneric, fmt.Sprintf("val-%d", i))
	}

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	writer := func(_ context.Context, _ RotatedRow) error {
		callCount++
		if callCount >= 2 {
			cancel()
		}
		return nil
	}

	result, err := RotateMasterKey(ctx, rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey should not return error, got: %v", err)
	}

	// At least 2 should have been processed before cancellation.
	if result.ReEncrypted < 2 {
		t.Errorf("ReEncrypted = %d, expected at least 2", result.ReEncrypted)
	}
	// Failed should account for the remaining rows.
	if result.Failed == 0 {
		t.Error("expected some rows to be marked as failed after cancellation")
	}
	total := result.AlreadyRotated + result.ReEncrypted + result.Failed
	if total != result.TotalCredentials {
		t.Errorf("accounting mismatch: %d + %d + %d = %d, want %d",
			result.AlreadyRotated, result.ReEncrypted, result.Failed, total, result.TotalCredentials)
	}
}

func TestRotateMasterKey_WriterReceivesCorrectFlags(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	rows := []RotationRow{
		encryptRow(t, prevEnc, "flag-1", CredentialTypeGeneric, "val-1"),
		encryptRow(t, prevEnc, "flag-2", CredentialTypeGeneric, "val-2"),
	}

	writer := func(_ context.Context, row RotatedRow) error {
		if !row.WasReEncrypted {
			// The writer should only be called for rows that need re-encryption.
			return fmt.Errorf("WasReEncrypted should be true for row %q", row.Name)
		}
		if row.NewEncryptedValue == "" {
			return fmt.Errorf("NewEncryptedValue should not be empty for row %q", row.Name)
		}
		if row.Name == "" {
			return fmt.Errorf("Name should not be empty")
		}
		return nil
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0; errors: %v", result.Failed, result.Errors)
	}
}

func TestRotateMasterKey_DuplicateNames(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	// Duplicate credential names — rotation should process each row
	// independently (this is the caller's responsibility to avoid, but
	// rotation should not crash).
	rows := []RotationRow{
		encryptRow(t, prevEnc, "dup", CredentialTypeGeneric, "first"),
		encryptRow(t, prevEnc, "dup", CredentialTypeGeneric, "second"),
	}

	writer, collected := collectingWriter()
	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, writer)
	if err != nil {
		t.Fatalf("RotateMasterKey failed: %v", err)
	}

	if result.TotalCredentials != 2 {
		t.Errorf("TotalCredentials = %d, want 2", result.TotalCredentials)
	}
	if result.ReEncrypted != 2 {
		t.Errorf("ReEncrypted = %d, want 2", result.ReEncrypted)
	}
	if len(*collected) != 2 {
		t.Fatalf("writer called %d times, want 2", len(*collected))
	}
}

// ---------------------------------------------------------------------------
// NeedsRotation — additional edge cases
// ---------------------------------------------------------------------------

func TestNeedsRotation_WhitespaceOnlyPreviousKey(t *testing.T) {
	env := fakeEnv(map[string]string{
		"CMM_CREDENTIAL_ENCRYPTION_KEY":          "new-key",
		"CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS": "   ",
	})
	// Whitespace is not empty, so NeedsRotation should return true.
	if !NeedsRotation(env) {
		t.Error("should need rotation when previous key is whitespace (non-empty)")
	}
}

func TestNeedsRotation_TabOnlyPreviousKey(t *testing.T) {
	env := fakeEnv(map[string]string{
		"CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS": "\t",
	})
	if !NeedsRotation(env) {
		t.Error("should need rotation when previous key is tab character (non-empty)")
	}
}

// ---------------------------------------------------------------------------
// RotationResult — additional field tests
// ---------------------------------------------------------------------------

func TestRotationResult_ErrorsMapContainsAllFailureNames(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)
	unknownEnc := rotationEncryptor(t, 99)

	rows := []RotationRow{
		encryptRow(t, unknownEnc, "fail-alpha", CredentialTypeGeneric, "v"),
		encryptRow(t, prevEnc, "ok-beta", CredentialTypeGeneric, "v"),
		encryptRow(t, unknownEnc, "fail-gamma", CredentialTypeGeneric, "v"),
	}

	result, err := RotateMasterKey(context.Background(), rows, newEnc, prevEnc, noopWriter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Errors) != 2 {
		t.Fatalf("Errors count = %d, want 2", len(result.Errors))
	}
	for _, name := range []string{"fail-alpha", "fail-gamma"} {
		e, ok := result.Errors[name]
		if !ok {
			t.Errorf("Errors should contain %q", name)
			continue
		}
		if !errors.Is(e, ErrRotationDecryptFailed) {
			t.Errorf("Errors[%q] should wrap ErrRotationDecryptFailed, got: %v", name, e)
		}
	}
	if _, ok := result.Errors["ok-beta"]; ok {
		t.Error("Errors should not contain 'ok-beta'")
	}
}

func TestRotationResult_Duration_EmptyRows(t *testing.T) {
	newEnc := rotationEncryptor(t, 10)
	prevEnc := rotationEncryptor(t, 20)

	result, err := RotateMasterKey(context.Background(), nil, newEnc, prevEnc, noopWriter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Duration < 0 {
		t.Error("Duration should be non-negative even for empty rows")
	}
}

// ---------------------------------------------------------------------------
// Sentinel errors — additional wrapping tests
// ---------------------------------------------------------------------------

func TestRotationSentinelErrors_CanBeUnwrappedFromWrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("context: %w", ErrRotationDecryptFailed)
	if !errors.Is(wrapped, ErrRotationDecryptFailed) {
		t.Error("errors.Is should find ErrRotationDecryptFailed through wrapping")
	}

	wrapped2 := fmt.Errorf("context: %w", ErrPreviousKeyRequired)
	if !errors.Is(wrapped2, ErrPreviousKeyRequired) {
		t.Error("errors.Is should find ErrPreviousKeyRequired through wrapping")
	}
}

// ---------------------------------------------------------------------------
// RotateCredentialRow — verify previous-key decryption does not accept new-key ciphertext
// ---------------------------------------------------------------------------

func TestRotateCredentialRow_PreviousKeyCannotDecryptNewKeyCiphertext(t *testing.T) {
	newEnc := rotationEncryptor(t, 1)
	prevEnc := rotationEncryptor(t, 2)

	// Row encrypted with new key. The new-key decrypt succeeds first,
	// so previous key is never tried. This verifies the short-circuit path.
	row := encryptRow(t, newEnc, "short-circuit", CredentialTypeGeneric, "val")

	rotated, err := RotateCredentialRow(row, newEnc, prevEnc)
	if err != nil {
		t.Fatalf("RotateCredentialRow failed: %v", err)
	}
	if rotated.WasReEncrypted {
		t.Error("should not be re-encrypted (new key should decrypt on first attempt)")
	}

	// Separately verify that prevEnc cannot decrypt the new-key ciphertext.
	aad, _ := BuildAAD(CredentialTypeGeneric, "short-circuit")
	_, err = prevEnc.Decrypt(row.EncryptedValue, aad)
	if err == nil {
		t.Error("previous encryptor should NOT be able to decrypt new-key ciphertext")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// getCredType looks up the credential type for a named row in the input slice.
func getCredType(t *testing.T, rows []RotationRow, name string) string {
	t.Helper()
	for _, r := range rows {
		if r.Name == name {
			return r.CredentialType
		}
	}
	t.Fatalf("row %q not found", name)
	return ""
}
