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
		CredentialTypeLDAPBindPassword,
		CredentialTypeSMTPPassword,
		CredentialTypeWebhookURL,
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
		CredentialTypeLDAPBindPassword,
		CredentialTypeSMTPPassword,
		CredentialTypeWebhookURL,
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
		CredentialTypeLDAPBindPassword,
		CredentialTypeSMTPPassword,
		CredentialTypeWebhookURL,
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
// webhook_url validation
// ---------------------------------------------------------------------------

func TestValidate_WebhookURL_HTTPS(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("https://hooks.slack.com/services/T00/B00/xxx"))
	if !result.Valid {
		t.Fatalf("expected HTTPS URL to pass: %v", result.Error)
	}
}

func TestValidate_WebhookURL_HTTP(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("http://internal-webhook.corp.example.com/notify"))
	if !result.Valid {
		t.Fatalf("expected HTTP URL to pass: %v", result.Error)
	}
}

func TestValidate_WebhookURL_WithPort(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("https://hooks.example.com:8443/webhook"))
	if !result.Valid {
		t.Fatalf("expected URL with port to pass: %v", result.Error)
	}
}

func TestValidate_WebhookURL_WithAuth(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("https://user:secret@hooks.example.com/webhook"))
	if !result.Valid {
		t.Fatalf("expected URL with auth to pass: %v", result.Error)
	}
}

func TestValidate_WebhookURL_WithQueryParams(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("https://hooks.example.com/webhook?token=abc123&channel=ops"))
	if !result.Valid {
		t.Fatalf("expected URL with query params to pass: %v", result.Error)
	}
}

func TestValidate_WebhookURL_FTPScheme(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("ftp://files.example.com/data"))
	if result.Valid {
		t.Fatal("expected FTP URL to fail validation")
	}
}

func TestValidate_WebhookURL_NoScheme(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("hooks.example.com/webhook"))
	if result.Valid {
		t.Fatal("expected URL without scheme to fail validation")
	}
}

func TestValidate_WebhookURL_NoHost(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("https:///path/only"))
	if result.Valid {
		t.Fatal("expected URL without host to fail validation")
	}
}

func TestValidate_WebhookURL_EmptyScheme(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("://example.com/webhook"))
	if result.Valid {
		t.Fatal("expected URL with empty scheme to fail validation")
	}
}

func TestValidate_WebhookURL_JustWhitespace(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("   \t\n  "))
	if result.Valid {
		t.Fatal("expected whitespace-only URL to fail validation")
	}
}

func TestValidate_WebhookURL_WithWhitespacePadding(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("  https://hooks.example.com/webhook  "))
	if !result.Valid {
		t.Fatalf("expected URL with whitespace padding to pass after trimming: %v", result.Error)
	}
}

func TestValidate_WebhookURL_MixedCaseScheme(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("HTTPS://hooks.example.com/webhook"))
	if !result.Valid {
		t.Fatalf("expected mixed-case HTTPS scheme to pass: %v", result.Error)
	}
}

func TestValidate_WebhookURL_Localhost(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("http://localhost:9090/webhook"))
	if !result.Valid {
		t.Fatalf("expected localhost URL to pass: %v", result.Error)
	}
}

func TestValidate_WebhookURL_IPAddress(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("http://192.168.1.100:8080/webhook"))
	if !result.Valid {
		t.Fatalf("expected IP address URL to pass: %v", result.Error)
	}
}

func TestValidate_WebhookURL_IPv6(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("http://[::1]:8080/webhook"))
	if !result.Valid {
		t.Fatalf("expected IPv6 URL to pass: %v", result.Error)
	}
}

func TestValidate_WebhookURL_NoMetadata(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeWebhookURL, []byte("https://example.com/hook"))
	if !result.Valid {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	// webhook_url validation should not produce metadata.
	if result.Metadata != nil {
		t.Fatalf("expected nil metadata for webhook_url, got %v", result.Metadata)
	}
}

// ---------------------------------------------------------------------------
// ldap_bind_password validation
// ---------------------------------------------------------------------------

func TestValidate_LDAPBindPassword_Valid(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeLDAPBindPassword, []byte("my-ldap-password"))
	if !result.Valid {
		t.Fatalf("expected non-empty LDAP password to pass: %v", result.Error)
	}
}

func TestValidate_LDAPBindPassword_SingleChar(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeLDAPBindPassword, []byte("x"))
	if !result.Valid {
		t.Fatalf("expected single-char LDAP password to pass: %v", result.Error)
	}
}

func TestValidate_LDAPBindPassword_SpecialChars(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeLDAPBindPassword, []byte("p@$$w0rd!#%^&*(){}[]|\\:\";<>,.?/~`"))
	if !result.Valid {
		t.Fatalf("expected LDAP password with special chars to pass: %v", result.Error)
	}
}

func TestValidate_LDAPBindPassword_Unicode(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeLDAPBindPassword, []byte("пароль密码パスワード"))
	if !result.Valid {
		t.Fatalf("expected Unicode LDAP password to pass: %v", result.Error)
	}
}

func TestValidate_LDAPBindPassword_NoMetadata(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeLDAPBindPassword, []byte("secret"))
	if !result.Valid {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Metadata != nil {
		t.Fatalf("expected nil metadata for ldap_bind_password, got %v", result.Metadata)
	}
}

// ---------------------------------------------------------------------------
// smtp_password validation
// ---------------------------------------------------------------------------

func TestValidate_SMTPPassword_Valid(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeSMTPPassword, []byte("smtp-secret-123"))
	if !result.Valid {
		t.Fatalf("expected non-empty SMTP password to pass: %v", result.Error)
	}
}

func TestValidate_SMTPPassword_AppPassword(t *testing.T) {
	// Google app passwords have a specific format.
	result := ValidateCredentialValue(CredentialTypeSMTPPassword, []byte("abcd efgh ijkl mnop"))
	if !result.Valid {
		t.Fatalf("expected app-password format to pass: %v", result.Error)
	}
}

func TestValidate_SMTPPassword_NoMetadata(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeSMTPPassword, []byte("secret"))
	if !result.Valid {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Metadata != nil {
		t.Fatalf("expected nil metadata for smtp_password, got %v", result.Metadata)
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
		CredentialTypeLDAPBindPassword,
		CredentialTypeSMTPPassword,
		CredentialTypeWebhookURL,
		CredentialTypeGeneric,
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
		{CredentialTypeLDAPBindPassword, "ldap_bind_password"},
		{CredentialTypeSMTPPassword, "smtp_password"},
		{CredentialTypeWebhookURL, "webhook_url"},
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
