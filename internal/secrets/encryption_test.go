package secrets

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// testMasterKey returns a valid Base64-encoded 32-byte master key for testing.
func testMasterKey(t *testing.T) string {
	t.Helper()
	// 32 bytes of deterministic test data, Base64-encoded.
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return base64.StdEncoding.EncodeToString(key)
}

// testMasterKeyAlt returns a second, different valid master key for testing
// scenarios that require two distinct keys (e.g. wrong-key decryption).
func testMasterKeyAlt(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 100)
	}
	return base64.StdEncoding.EncodeToString(key)
}

// ---------------------------------------------------------------------------
// NewEncryptor tests
// ---------------------------------------------------------------------------

func TestNewEncryptor_ValidKey(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor returned unexpected error: %v", err)
	}
	defer enc.Close()

	if enc.derivedKey == nil {
		t.Fatal("derived key is nil after successful creation")
	}
	if len(enc.derivedKey) != 32 {
		t.Fatalf("derived key length = %d, want 32", len(enc.derivedKey))
	}
}

func TestNewEncryptor_EmptyKey(t *testing.T) {
	_, err := NewEncryptor("")
	if err != ErrMasterKeyRequired {
		t.Fatalf("got error %v, want ErrMasterKeyRequired", err)
	}
}

func TestNewEncryptor_InvalidBase64(t *testing.T) {
	_, err := NewEncryptor("not!valid!base64!@#$%")
	if err != ErrMasterKeyInvalidBase64 {
		t.Fatalf("got error %v, want ErrMasterKeyInvalidBase64", err)
	}
}

func TestNewEncryptor_KeyTooShort(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16)) // 16 bytes, need 32
	_, err := NewEncryptor(shortKey)
	if err != ErrMasterKeyTooShort {
		t.Fatalf("got error %v, want ErrMasterKeyTooShort", err)
	}
}

func TestNewEncryptor_ExactlyMinimumLength(t *testing.T) {
	exactKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	enc, err := NewEncryptor(exactKey)
	if err != nil {
		t.Fatalf("NewEncryptor rejected a 32-byte key: %v", err)
	}
	enc.Close()
}

func TestNewEncryptor_LongerThanMinimum(t *testing.T) {
	longKey := base64.StdEncoding.EncodeToString(make([]byte, 64))
	enc, err := NewEncryptor(longKey)
	if err != nil {
		t.Fatalf("NewEncryptor rejected a 64-byte key: %v", err)
	}
	enc.Close()
}

func TestNewEncryptor_URLSafeBase64(t *testing.T) {
	// Some operators may use URL-safe Base64. The encryptor should
	// handle standard encoding but fall back to raw standard encoding.
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 50)
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)
	enc, err := NewEncryptor(encoded)
	if err != nil {
		t.Fatalf("NewEncryptor rejected raw-standard Base64 key: %v", err)
	}
	enc.Close()
}

// ---------------------------------------------------------------------------
// Close / zeroing tests
// ---------------------------------------------------------------------------

func TestEncryptor_Close_ZerosDerivedKey(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	// Keep a reference to the underlying slice to check after Close.
	keyRef := enc.derivedKey

	enc.Close()

	if enc.derivedKey != nil {
		t.Fatal("derivedKey should be nil after Close")
	}
	if !IsZeroed(keyRef) {
		t.Fatal("derived key memory was not zeroed after Close")
	}
}

func TestEncryptor_Close_Idempotent(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	enc.Close()
	enc.Close() // should not panic
}

func TestEncryptor_Close_NilReceiver(t *testing.T) {
	var enc *Encryptor
	enc.Close() // should not panic
}

// ---------------------------------------------------------------------------
// BuildAAD tests
// ---------------------------------------------------------------------------

func TestBuildAAD_Valid(t *testing.T) {
	aad, err := BuildAAD("chef_client_key", "myorg-production-key")
	if err != nil {
		t.Fatalf("BuildAAD: %v", err)
	}
	expected := "chef_client_key:myorg-production-key"
	if string(aad) != expected {
		t.Fatalf("AAD = %q, want %q", string(aad), expected)
	}
}

func TestBuildAAD_EmptyType(t *testing.T) {
	_, err := BuildAAD("", "some-name")
	if err != ErrAADRequired {
		t.Fatalf("got error %v, want ErrAADRequired", err)
	}
}

func TestBuildAAD_EmptyName(t *testing.T) {
	_, err := BuildAAD("chef_client_key", "")
	if err != ErrAADRequired {
		t.Fatalf("got error %v, want ErrAADRequired", err)
	}
}

func TestBuildAAD_BothEmpty(t *testing.T) {
	_, err := BuildAAD("", "")
	if err != ErrAADRequired {
		t.Fatalf("got error %v, want ErrAADRequired", err)
	}
}

// ---------------------------------------------------------------------------
// Encrypt / Decrypt round-trip tests
// ---------------------------------------------------------------------------

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	plaintext := []byte("-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAK...\n-----END RSA PRIVATE KEY-----\n")
	aad, _ := BuildAAD("chef_client_key", "myorg-key")

	ciphertext, err := enc.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if ciphertext == "" {
		t.Fatal("Encrypt returned empty ciphertext")
	}

	// Ciphertext should be in <nonce_hex>:<ciphertext_hex> format.
	parts := strings.SplitN(ciphertext, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("ciphertext format invalid, expected nonce:ciphertext, got %q", ciphertext)
	}

	decrypted, err := enc.Decrypt(ciphertext, aad)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	defer ZeroBytes(decrypted)

	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted text does not match original.\ngot:  %q\nwant: %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	aad, _ := BuildAAD("generic", "empty-test")

	ciphertext, err := enc.Encrypt([]byte{}, aad)
	if err != nil {
		t.Fatalf("Encrypt with empty plaintext: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext, aad)
	if err != nil {
		t.Fatalf("Decrypt of empty plaintext: %v", err)
	}
	defer ZeroBytes(decrypted)

	if len(decrypted) != 0 {
		t.Fatalf("expected empty decrypted result, got %d bytes", len(decrypted))
	}
}

func TestEncryptDecrypt_LargePlaintext(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	// 1 MB of data
	plaintext := make([]byte, 1024*1024)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}
	aad, _ := BuildAAD("generic", "large-value")

	ciphertext, err := enc.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt large plaintext: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext, aad)
	if err != nil {
		t.Fatalf("Decrypt large plaintext: %v", err)
	}
	defer ZeroBytes(decrypted)

	if len(decrypted) != len(plaintext) {
		t.Fatalf("decrypted length = %d, want %d", len(decrypted), len(plaintext))
	}
	for i := range plaintext {
		if decrypted[i] != plaintext[i] {
			t.Fatalf("mismatch at byte %d: got %d, want %d", i, decrypted[i], plaintext[i])
		}
	}
}

func TestEncryptDecrypt_NilAAD(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	plaintext := []byte("some secret value")

	// Encrypt with nil AAD should work (GCM allows nil AAD).
	ciphertext, err := enc.Encrypt(plaintext, nil)
	if err != nil {
		t.Fatalf("Encrypt with nil AAD: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext, nil)
	if err != nil {
		t.Fatalf("Decrypt with nil AAD: %v", err)
	}
	defer ZeroBytes(decrypted)

	if string(decrypted) != string(plaintext) {
		t.Fatalf("round-trip with nil AAD failed")
	}
}

func TestEncryptDecrypt_SpecialCharactersInPlaintext(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	// Plaintext with newlines, null bytes, unicode, and colons (to test
	// that the colon separator in at-rest format doesn't cause issues).
	plaintext := []byte("line1\nline2\x00null\ttab\r\ncrlf:colon:more:colons\U0001F512")
	aad, _ := BuildAAD("generic", "special-chars")

	ciphertext, err := enc.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext, aad)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	defer ZeroBytes(decrypted)

	if string(decrypted) != string(plaintext) {
		t.Fatal("round-trip with special characters failed")
	}
}

// ---------------------------------------------------------------------------
// Nonce uniqueness tests
// ---------------------------------------------------------------------------

func TestEncrypt_NonceUniqueness(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	plaintext := []byte("identical value")
	aad, _ := BuildAAD("generic", "nonce-test")

	// Encrypt the same plaintext multiple times.
	const iterations = 50
	ciphertexts := make(map[string]bool, iterations)
	nonces := make(map[string]bool, iterations)

	for i := 0; i < iterations; i++ {
		ct, err := enc.Encrypt(plaintext, aad)
		if err != nil {
			t.Fatalf("Encrypt iteration %d: %v", i, err)
		}

		// Each ciphertext string should be unique.
		if ciphertexts[ct] {
			t.Fatalf("duplicate ciphertext on iteration %d", i)
		}
		ciphertexts[ct] = true

		// Extract and check the nonce portion.
		parts := strings.SplitN(ct, ":", 2)
		if nonces[parts[0]] {
			t.Fatalf("duplicate nonce on iteration %d: %s", i, parts[0])
		}
		nonces[parts[0]] = true

		// Each unique ciphertext should still decrypt correctly.
		decrypted, err := enc.Decrypt(ct, aad)
		if err != nil {
			t.Fatalf("Decrypt of iteration %d: %v", i, err)
		}
		if string(decrypted) != string(plaintext) {
			t.Fatalf("decrypted mismatch on iteration %d", i)
		}
		ZeroBytes(decrypted)
	}
}

// ---------------------------------------------------------------------------
// AAD mismatch (row-swap prevention) tests
// ---------------------------------------------------------------------------

func TestDecrypt_AADMismatch_DifferentName(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	plaintext := []byte("my secret key")
	aadOriginal, _ := BuildAAD("chef_client_key", "org-a-key")
	aadSwapped, _ := BuildAAD("chef_client_key", "org-b-key")

	ciphertext, err := enc.Encrypt(plaintext, aadOriginal)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Attempting to decrypt with a different name should fail.
	_, err = enc.Decrypt(ciphertext, aadSwapped)
	if err == nil {
		t.Fatal("expected decryption to fail with mismatched AAD name, but it succeeded")
	}
	if err != ErrDecryptionFailed {
		t.Fatalf("got error %v, want ErrDecryptionFailed", err)
	}
}

func TestDecrypt_AADMismatch_DifferentType(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	plaintext := []byte("some password")
	aadOriginal, _ := BuildAAD("ldap_bind_password", "ldap-pw")
	aadSwapped, _ := BuildAAD("smtp_password", "ldap-pw")

	ciphertext, err := enc.Encrypt(plaintext, aadOriginal)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = enc.Decrypt(ciphertext, aadSwapped)
	if err == nil {
		t.Fatal("expected decryption to fail with mismatched AAD type, but it succeeded")
	}
	if err != ErrDecryptionFailed {
		t.Fatalf("got error %v, want ErrDecryptionFailed", err)
	}
}

func TestDecrypt_AADMismatch_NilVsNonNil(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	plaintext := []byte("secret")

	// Encrypt with AAD, decrypt without.
	aad, _ := BuildAAD("generic", "test")
	ciphertext, err := enc.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = enc.Decrypt(ciphertext, nil)
	if err == nil {
		t.Fatal("expected decryption to fail when AAD is nil but was non-nil during encryption")
	}

	// Encrypt without AAD, decrypt with.
	ciphertext2, err := enc.Encrypt(plaintext, nil)
	if err != nil {
		t.Fatalf("Encrypt with nil AAD: %v", err)
	}
	_, err = enc.Decrypt(ciphertext2, aad)
	if err == nil {
		t.Fatal("expected decryption to fail when AAD is non-nil but was nil during encryption")
	}
}

// ---------------------------------------------------------------------------
// Tampered ciphertext tests
// ---------------------------------------------------------------------------

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	plaintext := []byte("important data")
	aad, _ := BuildAAD("generic", "tamper-test")

	ciphertext, err := enc.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	parts := strings.SplitN(ciphertext, ":", 2)
	ctBytes, _ := hex.DecodeString(parts[1])

	// Flip a bit in the middle of the ciphertext.
	ctBytes[len(ctBytes)/2] ^= 0xFF
	tampered := parts[0] + ":" + hex.EncodeToString(ctBytes)

	_, err = enc.Decrypt(tampered, aad)
	if err == nil {
		t.Fatal("expected decryption to fail with tampered ciphertext")
	}
	if err != ErrDecryptionFailed {
		t.Fatalf("got error %v, want ErrDecryptionFailed", err)
	}
}

func TestDecrypt_TamperedNonce(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	plaintext := []byte("important data")
	aad, _ := BuildAAD("generic", "tamper-nonce-test")

	ciphertext, err := enc.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	parts := strings.SplitN(ciphertext, ":", 2)
	nonceBytes, _ := hex.DecodeString(parts[0])
	nonceBytes[0] ^= 0xFF
	tampered := hex.EncodeToString(nonceBytes) + ":" + parts[1]

	_, err = enc.Decrypt(tampered, aad)
	if err == nil {
		t.Fatal("expected decryption to fail with tampered nonce")
	}
	if err != ErrDecryptionFailed {
		t.Fatalf("got error %v, want ErrDecryptionFailed", err)
	}
}

// ---------------------------------------------------------------------------
// Wrong key tests
// ---------------------------------------------------------------------------

func TestDecrypt_WrongKey(t *testing.T) {
	enc1, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor (key 1): %v", err)
	}
	defer enc1.Close()

	enc2, err := NewEncryptor(testMasterKeyAlt(t))
	if err != nil {
		t.Fatalf("NewEncryptor (key 2): %v", err)
	}
	defer enc2.Close()

	plaintext := []byte("secret value")
	aad, _ := BuildAAD("generic", "wrong-key-test")

	ciphertext, err := enc1.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt with key 1: %v", err)
	}

	_, err = enc2.Decrypt(ciphertext, aad)
	if err == nil {
		t.Fatal("expected decryption to fail with wrong key")
	}
	if err != ErrDecryptionFailed {
		t.Fatalf("got error %v, want ErrDecryptionFailed", err)
	}
}

func TestDecrypt_SameKeyDifferentEncryptorInstances(t *testing.T) {
	masterKey := testMasterKey(t)

	enc1, err := NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor (instance 1): %v", err)
	}
	defer enc1.Close()

	enc2, err := NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor (instance 2): %v", err)
	}
	defer enc2.Close()

	plaintext := []byte("cross-instance test")
	aad, _ := BuildAAD("generic", "cross-instance")

	ciphertext, err := enc1.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt with instance 1: %v", err)
	}

	// A different Encryptor created with the same master key should
	// produce the same derived key and be able to decrypt.
	decrypted, err := enc2.Decrypt(ciphertext, aad)
	if err != nil {
		t.Fatalf("Decrypt with instance 2: %v", err)
	}
	defer ZeroBytes(decrypted)

	if string(decrypted) != string(plaintext) {
		t.Fatal("cross-instance round-trip failed")
	}
}

// ---------------------------------------------------------------------------
// Invalid ciphertext format tests
// ---------------------------------------------------------------------------

func TestDecrypt_InvalidFormat_NoSeparator(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	_, err = enc.Decrypt("deadbeefdeadbeefdeadbeef", nil)
	if err == nil {
		t.Fatal("expected error for ciphertext without separator")
	}
	if !strings.Contains(err.Error(), "invalid ciphertext format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecrypt_InvalidFormat_BadNonceHex(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	_, err = enc.Decrypt("not_hex_at_all!:deadbeef", nil)
	if err == nil {
		t.Fatal("expected error for invalid nonce hex")
	}
	if !strings.Contains(err.Error(), "invalid nonce hex") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecrypt_InvalidFormat_BadCiphertextHex(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	nonce := hex.EncodeToString(make([]byte, 12))
	_, err = enc.Decrypt(nonce+":not_valid_hex!!!", nil)
	if err == nil {
		t.Fatal("expected error for invalid ciphertext hex")
	}
	if !strings.Contains(err.Error(), "invalid ciphertext hex") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecrypt_InvalidFormat_WrongNonceLength(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	shortNonce := hex.EncodeToString(make([]byte, 8)) // 8 bytes, need 12
	ct := hex.EncodeToString(make([]byte, 32))
	_, err = enc.Decrypt(shortNonce+":"+ct, nil)
	if err == nil {
		t.Fatal("expected error for wrong nonce length")
	}
	if !strings.Contains(err.Error(), "nonce must be 12 bytes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecrypt_InvalidFormat_EmptyString(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	_, err = enc.Decrypt("", nil)
	if err == nil {
		t.Fatal("expected error for empty ciphertext string")
	}
}

// ---------------------------------------------------------------------------
// Nil / closed encryptor tests
// ---------------------------------------------------------------------------

func TestEncrypt_NilEncryptor(t *testing.T) {
	var enc *Encryptor
	_, err := enc.Encrypt([]byte("data"), nil)
	if err != ErrMasterKeyRequired {
		t.Fatalf("got error %v, want ErrMasterKeyRequired", err)
	}
}

func TestDecrypt_NilEncryptor(t *testing.T) {
	var enc *Encryptor
	_, err := enc.Decrypt("something:else", nil)
	if err != ErrMasterKeyRequired {
		t.Fatalf("got error %v, want ErrMasterKeyRequired", err)
	}
}

func TestEncrypt_ClosedEncryptor(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	enc.Close()

	_, err = enc.Encrypt([]byte("data"), nil)
	if err != ErrMasterKeyRequired {
		t.Fatalf("got error %v, want ErrMasterKeyRequired", err)
	}
}

func TestDecrypt_ClosedEncryptor(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	enc.Close()

	_, err = enc.Decrypt("something:else", nil)
	if err != ErrMasterKeyRequired {
		t.Fatalf("got error %v, want ErrMasterKeyRequired", err)
	}
}

// ---------------------------------------------------------------------------
// At-rest format structure tests
// ---------------------------------------------------------------------------

func TestEncrypt_AtRestFormat(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	ciphertext, err := enc.Encrypt([]byte("test"), nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	parts := strings.SplitN(ciphertext, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("expected nonce:ciphertext format, got %q", ciphertext)
	}

	// Nonce part should be exactly 24 hex characters (12 bytes).
	if len(parts[0]) != 24 {
		t.Fatalf("nonce hex length = %d, want 24", len(parts[0]))
	}

	// Both parts should be valid hex.
	if _, err := hex.DecodeString(parts[0]); err != nil {
		t.Fatalf("nonce is not valid hex: %v", err)
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		t.Fatalf("ciphertext is not valid hex: %v", err)
	}

	// Ciphertext should be longer than the plaintext due to the GCM tag
	// (16 bytes = 32 hex chars).
	ctBytes, _ := hex.DecodeString(parts[1])
	if len(ctBytes) < 16 {
		t.Fatalf("ciphertext too short to contain GCM tag: %d bytes", len(ctBytes))
	}
}

// ---------------------------------------------------------------------------
// Ciphertext does not contain plaintext
// ---------------------------------------------------------------------------

func TestEncrypt_CiphertextDoesNotContainPlaintext(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	plaintext := []byte("UNIQUE_MARKER_STRING_12345")
	aad, _ := BuildAAD("generic", "leak-test")

	ciphertext, err := enc.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if strings.Contains(ciphertext, string(plaintext)) {
		t.Fatal("ciphertext contains plaintext — encryption may not be working")
	}

	// Also check the hex-encoded version of the plaintext.
	plaintextHex := hex.EncodeToString(plaintext)
	if strings.Contains(ciphertext, plaintextHex) {
		t.Fatal("ciphertext contains hex-encoded plaintext")
	}
}

// ---------------------------------------------------------------------------
// DecodeMasterKey tests
// ---------------------------------------------------------------------------

func TestDecodeMasterKey_Valid(t *testing.T) {
	key := testMasterKey(t)
	decoded, err := DecodeMasterKey(key)
	if err != nil {
		t.Fatalf("DecodeMasterKey: %v", err)
	}
	defer ZeroBytes(decoded)

	if len(decoded) < 32 {
		t.Fatalf("decoded key length = %d, want >= 32", len(decoded))
	}
}

func TestDecodeMasterKey_Empty(t *testing.T) {
	_, err := DecodeMasterKey("")
	if err != ErrMasterKeyRequired {
		t.Fatalf("got error %v, want ErrMasterKeyRequired", err)
	}
}

func TestDecodeMasterKey_InvalidBase64(t *testing.T) {
	_, err := DecodeMasterKey("not!valid!base64!@#$%")
	if err != ErrMasterKeyInvalidBase64 {
		t.Fatalf("got error %v, want ErrMasterKeyInvalidBase64", err)
	}
}

func TestDecodeMasterKey_TooShort(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 10))
	_, err := DecodeMasterKey(shortKey)
	if err != ErrMasterKeyTooShort {
		t.Fatalf("got error %v, want ErrMasterKeyTooShort", err)
	}
}

// ---------------------------------------------------------------------------
// HKDF determinism test — same master key always produces the same derived key
// ---------------------------------------------------------------------------

func TestNewEncryptor_DeterministicDerivation(t *testing.T) {
	masterKey := testMasterKey(t)

	enc1, err := NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor 1: %v", err)
	}
	key1 := make([]byte, len(enc1.derivedKey))
	copy(key1, enc1.derivedKey)
	enc1.Close()

	enc2, err := NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor 2: %v", err)
	}
	key2 := make([]byte, len(enc2.derivedKey))
	copy(key2, enc2.derivedKey)
	enc2.Close()

	if len(key1) != len(key2) {
		t.Fatalf("derived key lengths differ: %d vs %d", len(key1), len(key2))
	}
	for i := range key1 {
		if key1[i] != key2[i] {
			t.Fatal("HKDF derivation is not deterministic — derived keys differ for the same master key")
		}
	}

	ZeroBytes(key1)
	ZeroBytes(key2)
}

// ---------------------------------------------------------------------------
// Different master keys produce different derived keys
// ---------------------------------------------------------------------------

func TestNewEncryptor_DifferentKeysDifferentDerivation(t *testing.T) {
	enc1, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor 1: %v", err)
	}
	key1 := make([]byte, len(enc1.derivedKey))
	copy(key1, enc1.derivedKey)
	enc1.Close()

	enc2, err := NewEncryptor(testMasterKeyAlt(t))
	if err != nil {
		t.Fatalf("NewEncryptor 2: %v", err)
	}
	key2 := make([]byte, len(enc2.derivedKey))
	copy(key2, enc2.derivedKey)
	enc2.Close()

	same := true
	for i := range key1 {
		if key1[i] != key2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different master keys produced the same derived key")
	}

	ZeroBytes(key1)
	ZeroBytes(key2)
}

// ---------------------------------------------------------------------------
// All credential types as AAD
// ---------------------------------------------------------------------------

func TestEncryptDecrypt_AllCredentialTypes(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	types := []string{
		"chef_client_key",
		"ldap_bind_password",
		"smtp_password",
		"webhook_url",
		"generic",
	}

	for _, ct := range types {
		t.Run(ct, func(t *testing.T) {
			plaintext := []byte("secret-for-" + ct)
			aad, err := BuildAAD(ct, "test-name")
			if err != nil {
				t.Fatalf("BuildAAD: %v", err)
			}

			ciphertext, err := enc.Encrypt(plaintext, aad)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}

			decrypted, err := enc.Decrypt(ciphertext, aad)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			defer ZeroBytes(decrypted)

			if string(decrypted) != string(plaintext) {
				t.Fatalf("round-trip failed for credential type %s", ct)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cross-type AAD rejection — a ciphertext encrypted with one type cannot
// be decrypted with a different type even when the name matches.
// ---------------------------------------------------------------------------

func TestDecrypt_CrossTypeAADRejection(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	name := "shared-name"
	types := []string{
		"chef_client_key",
		"ldap_bind_password",
		"smtp_password",
		"webhook_url",
		"generic",
	}

	// Encrypt with each type.
	ciphertexts := make(map[string]string)
	for _, ct := range types {
		aad, _ := BuildAAD(ct, name)
		ciphertext, err := enc.Encrypt([]byte("value-"+ct), aad)
		if err != nil {
			t.Fatalf("Encrypt for %s: %v", ct, err)
		}
		ciphertexts[ct] = ciphertext
	}

	// Try decrypting each ciphertext with every *other* type. All should fail.
	for encType, ciphertext := range ciphertexts {
		for _, decType := range types {
			if decType == encType {
				continue
			}
			aad, _ := BuildAAD(decType, name)
			_, err := enc.Decrypt(ciphertext, aad)
			if err == nil {
				t.Fatalf("decrypting %s ciphertext with %s AAD should have failed", encType, decType)
			}
		}
	}
}
