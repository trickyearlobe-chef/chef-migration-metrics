package secrets

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

// ---------------------------------------------------------------------------
// Helper: generate a PEM-encoded RSA private key for testing.
// ---------------------------------------------------------------------------

func generateTestRSAKeyPKCS1(t *testing.T, bits int) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("failed to generate %d-bit RSA key: %v", bits, err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

func generateTestRSAKeyPKCS8(t *testing.T, bits int) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("failed to generate %d-bit RSA key: %v", bits, err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal PKCS#8: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})
}

// ---------------------------------------------------------------------------
// IsValidCredentialType tests
// ---------------------------------------------------------------------------

func TestIsValidCredentialType_AllKnownTypes(t *testing.T) {
	knownTypes := []string{
		CredentialTypeChefClientKey,
		CredentialTypeGeneric,
	}
	for _, ct := range knownTypes {
		if !IsValidCredentialType(ct) {
			t.Errorf("IsValidCredentialType(%q) = false, want true", ct)
		}
	}
}

func TestIsValidCredentialType_Unknown(t *testing.T) {
	unknowns := []string{
		"",
		"unknown",
		"Chef_Client_Key",
		"CHEF_CLIENT_KEY",
		"ssh_key",
		"api_token",
	}
	for _, ct := range unknowns {
		if IsValidCredentialType(ct) {
			t.Errorf("IsValidCredentialType(%q) = true, want false", ct)
		}
	}
}

// ---------------------------------------------------------------------------
// ValidateCredentialValue: unrecognised type
// ---------------------------------------------------------------------------

func TestValidate_UnrecognisedType(t *testing.T) {
	result := ValidateCredentialValue("not_a_real_type", []byte("some value"))
	if result.Valid {
		t.Fatal("expected validation to fail for unrecognised type")
	}
	if result.Error == nil {
		t.Fatal("expected non-nil error")
	}
}

// ---------------------------------------------------------------------------
// ValidateCredentialValue: empty value (all types)
// ---------------------------------------------------------------------------

func TestValidate_EmptyValue_AllTypes(t *testing.T) {
	for _, ct := range []string{
		CredentialTypeChefClientKey,
		CredentialTypeGeneric,
	} {
		t.Run(ct, func(t *testing.T) {
			result := ValidateCredentialValue(ct, []byte{})
			if result.Valid {
				t.Fatalf("expected validation to fail for empty value with type %q", ct)
			}
			if result.Error != ErrEmptyValue {
				t.Fatalf("got error %v, want ErrEmptyValue", result.Error)
			}
		})
	}
}

func TestValidate_NilValue_AllTypes(t *testing.T) {
	for _, ct := range []string{
		CredentialTypeChefClientKey,
		CredentialTypeGeneric,
	} {
		t.Run(ct, func(t *testing.T) {
			result := ValidateCredentialValue(ct, nil)
			if result.Valid {
				t.Fatalf("expected validation to fail for nil value with type %q", ct)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// chef_client_key validation
// ---------------------------------------------------------------------------

func TestValidate_ChefClientKey_PKCS1_2048(t *testing.T) {
	pemData := generateTestRSAKeyPKCS1(t, 2048)
	result := ValidateCredentialValue(CredentialTypeChefClientKey, pemData)

	if !result.Valid {
		t.Fatalf("expected valid PKCS#1 2048-bit key to pass: %v", result.Error)
	}
	if result.Metadata == nil {
		t.Fatal("expected metadata to be populated")
	}
	if result.Metadata["key_format"] != "pkcs1" {
		t.Fatalf("key_format = %v, want pkcs1", result.Metadata["key_format"])
	}
	bits, ok := result.Metadata["bits"].(int)
	if !ok {
		t.Fatalf("bits metadata is not an int: %T", result.Metadata["bits"])
	}
	if bits != 2048 {
		t.Fatalf("bits = %d, want 2048", bits)
	}
}

func TestValidate_ChefClientKey_PKCS1_4096(t *testing.T) {
	pemData := generateTestRSAKeyPKCS1(t, 4096)
	result := ValidateCredentialValue(CredentialTypeChefClientKey, pemData)

	if !result.Valid {
		t.Fatalf("expected valid PKCS#1 4096-bit key to pass: %v", result.Error)
	}
	bits := result.Metadata["bits"].(int)
	if bits != 4096 {
		t.Fatalf("bits = %d, want 4096", bits)
	}
}

func TestValidate_ChefClientKey_PKCS8(t *testing.T) {
	pemData := generateTestRSAKeyPKCS8(t, 2048)
	result := ValidateCredentialValue(CredentialTypeChefClientKey, pemData)

	if !result.Valid {
		t.Fatalf("expected valid PKCS#8 key to pass: %v", result.Error)
	}
	if result.Metadata["key_format"] != "pkcs8" {
		t.Fatalf("key_format = %v, want pkcs8", result.Metadata["key_format"])
	}
}

func TestValidate_ChefClientKey_NoPEMBlock(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeChefClientKey, []byte("this is not PEM data"))
	if result.Valid {
		t.Fatal("expected validation to fail for non-PEM data")
	}
	if result.Error == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestValidate_ChefClientKey_WrongPEMType(t *testing.T) {
	// Create a PEM block with an unexpected type.
	block := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a real certificate"),
	}
	pemData := pem.EncodeToMemory(block)

	result := ValidateCredentialValue(CredentialTypeChefClientKey, pemData)
	if result.Valid {
		t.Fatal("expected validation to fail for CERTIFICATE PEM type")
	}
}

func TestValidate_ChefClientKey_CorruptPKCS1(t *testing.T) {
	// Valid PEM wrapper, but garbage DER content.
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: []byte("definitely not valid ASN.1 DER"),
	}
	pemData := pem.EncodeToMemory(block)

	result := ValidateCredentialValue(CredentialTypeChefClientKey, pemData)
	if result.Valid {
		t.Fatal("expected validation to fail for corrupt PKCS#1 data")
	}
}

func TestValidate_ChefClientKey_CorruptPKCS8(t *testing.T) {
	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: []byte("definitely not valid PKCS#8 DER"),
	}
	pemData := pem.EncodeToMemory(block)

	result := ValidateCredentialValue(CredentialTypeChefClientKey, pemData)
	if result.Valid {
		t.Fatal("expected validation to fail for corrupt PKCS#8 data")
	}
}

func TestValidate_ChefClientKey_PKCS8NotRSA(t *testing.T) {
	// Generate an EC key and wrap it in PKCS#8 — should be rejected
	// because we only support RSA for Chef API signing.
	ecPEM := generateTestECKeyPKCS8(t)

	result := ValidateCredentialValue(CredentialTypeChefClientKey, ecPEM)
	if result.Valid {
		t.Fatal("expected validation to fail for EC key in PKCS#8 format")
	}
}

func generateTestECKeyPKCS8(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate EC key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal EC key to PKCS#8: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})
}

func TestValidate_ChefClientKey_WithTrailingNewline(t *testing.T) {
	pemData := generateTestRSAKeyPKCS1(t, 2048)
	// Add extra trailing whitespace/newlines (common in real-world PEM files).
	pemData = append(pemData, '\n', '\n', ' ', '\n')

	result := ValidateCredentialValue(CredentialTypeChefClientKey, pemData)
	if !result.Valid {
		t.Fatalf("expected key with trailing whitespace to pass: %v", result.Error)
	}
}

func TestValidate_ChefClientKey_WithLeadingWhitespace(t *testing.T) {
	pemData := generateTestRSAKeyPKCS1(t, 2048)
	// Prepend whitespace (some operators paste keys with leading spaces).
	pemData = append([]byte("  \n"), pemData...)

	result := ValidateCredentialValue(CredentialTypeChefClientKey, pemData)
	if !result.Valid {
		t.Fatalf("expected key with leading whitespace to pass: %v", result.Error)
	}
}

// ---------------------------------------------------------------------------
// generic validation
// ---------------------------------------------------------------------------

func TestValidate_Generic_Valid(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeGeneric, []byte("any arbitrary value"))
	if !result.Valid {
		t.Fatalf("expected non-empty generic value to pass: %v", result.Error)
	}
}

func TestValidate_Generic_BinaryData(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	result := ValidateCredentialValue(CredentialTypeGeneric, data)
	if !result.Valid {
		t.Fatalf("expected binary data to pass generic validation: %v", result.Error)
	}
}

func TestValidate_Generic_SingleByte(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeGeneric, []byte{0x00})
	// A single null byte is still non-empty (len == 1).
	if !result.Valid {
		t.Fatalf("expected single null byte to pass generic validation: %v", result.Error)
	}
}

func TestValidate_Generic_NoMetadata(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeGeneric, []byte("value"))
	if !result.Valid {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Metadata != nil {
		t.Fatalf("expected nil metadata for generic, got %v", result.Metadata)
	}
}

// ---------------------------------------------------------------------------
// ValidCredentialTypes map completeness
// ---------------------------------------------------------------------------

func TestValidCredentialTypes_ContainsAllConstants(t *testing.T) {
	expected := []string{
		CredentialTypeChefClientKey,
		CredentialTypeGeneric,
		CredentialTypeDatabaseURL,
	}
	for _, ct := range expected {
		if !ValidCredentialTypes[ct] {
			t.Errorf("ValidCredentialTypes missing %q", ct)
		}
	}
	if len(ValidCredentialTypes) != len(expected) {
		t.Errorf("ValidCredentialTypes has %d entries, want %d", len(ValidCredentialTypes), len(expected))
	}
}

// ---------------------------------------------------------------------------
// Credential type constants match expected string values
// ---------------------------------------------------------------------------

func TestCredentialTypeConstants(t *testing.T) {
	tests := []struct {
		constant string
		expected string
	}{
		{CredentialTypeChefClientKey, "chef_client_key"},
		{CredentialTypeDatabaseURL, "database_url"},
		{CredentialTypeGeneric, "generic"},
	}
	for _, tt := range tests {
		if tt.constant != tt.expected {
			t.Errorf("constant value = %q, want %q", tt.constant, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// ValidationResult struct behaviour
// ---------------------------------------------------------------------------

func TestValidationResult_ValidHasNoError(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeGeneric, []byte("ok"))
	if !result.Valid {
		t.Fatal("expected Valid to be true")
	}
	if result.Error != nil {
		t.Fatalf("expected nil error for valid result, got %v", result.Error)
	}
}

func TestValidationResult_InvalidHasError(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeChefClientKey, []byte("not a key"))
	if result.Valid {
		t.Fatal("expected Valid to be false")
	}
	if result.Error == nil {
		t.Fatal("expected non-nil error for invalid result")
	}
}
