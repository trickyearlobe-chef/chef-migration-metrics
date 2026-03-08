package secrets

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Sentinel errors returned by credential validation.
var (
	// ErrInvalidCredentialType is returned when a credential_type value is
	// not one of the recognised types.
	ErrInvalidCredentialType = errors.New("secrets: unrecognised credential type")

	// ErrEmptyValue is returned when a credential value is empty.
	ErrEmptyValue = errors.New("secrets: credential value must not be empty")

	// ErrInvalidPEMKey is returned when a chef_client_key value is not a
	// valid PEM-encoded RSA private key.
	ErrInvalidPEMKey = errors.New("secrets: value is not a valid PEM-encoded RSA private key")

	// ErrInvalidWebhookURL is returned when a webhook_url value is not a
	// valid HTTP or HTTPS URL.
	ErrInvalidWebhookURL = errors.New("secrets: value is not a valid http or https URL")
)

// Known credential type constants. These match the CHECK constraint on the
// credentials table and the credential_type values used throughout the
// application.
const (
	CredentialTypeChefClientKey    = "chef_client_key"
	CredentialTypeLDAPBindPassword = "ldap_bind_password"
	CredentialTypeSMTPPassword     = "smtp_password"
	CredentialTypeWebhookURL       = "webhook_url"
	CredentialTypeGeneric          = "generic"
)

// ValidCredentialTypes is the set of all recognised credential types.
var ValidCredentialTypes = map[string]bool{
	CredentialTypeChefClientKey:    true,
	CredentialTypeLDAPBindPassword: true,
	CredentialTypeSMTPPassword:     true,
	CredentialTypeWebhookURL:       true,
	CredentialTypeGeneric:          true,
}

// ValidationResult holds the outcome of a credential value validation,
// including any metadata that was extracted during validation (e.g. RSA
// key size for chef_client_key).
type ValidationResult struct {
	// Valid is true if the credential value passes type-specific validation.
	Valid bool

	// Error is the validation error, if any. Nil when Valid is true.
	Error error

	// Metadata contains non-sensitive metadata extracted during validation.
	// For chef_client_key this includes key_format and bits. For other
	// types this may be nil.
	Metadata map[string]any
}

// ValidateCredentialValue validates a credential value according to its type.
// It returns a ValidationResult that indicates whether the value is valid,
// any validation error, and any metadata extracted during validation.
//
// The plaintext value is inspected but not modified. The caller is responsible
// for zeroing the value after this function returns.
func ValidateCredentialValue(credentialType string, value []byte) ValidationResult {
	if !ValidCredentialTypes[credentialType] {
		return ValidationResult{
			Valid: false,
			Error: fmt.Errorf("%w: %q", ErrInvalidCredentialType, credentialType),
		}
	}

	if len(value) == 0 {
		return ValidationResult{
			Valid: false,
			Error: ErrEmptyValue,
		}
	}

	switch credentialType {
	case CredentialTypeChefClientKey:
		return validateChefClientKey(value)
	case CredentialTypeWebhookURL:
		return validateWebhookURL(value)
	case CredentialTypeLDAPBindPassword:
		return validateNonEmpty(value)
	case CredentialTypeSMTPPassword:
		return validateNonEmpty(value)
	case CredentialTypeGeneric:
		return validateNonEmpty(value)
	default:
		// This should be unreachable because of the ValidCredentialTypes
		// check above, but handle it defensively.
		return ValidationResult{
			Valid: false,
			Error: fmt.Errorf("%w: %q", ErrInvalidCredentialType, credentialType),
		}
	}
}

// IsValidCredentialType returns true if the given string is a recognised
// credential type.
func IsValidCredentialType(credentialType string) bool {
	return ValidCredentialTypes[credentialType]
}

// validateChefClientKey validates that the value is a PEM-encoded RSA
// private key. It extracts the key format (PKCS#1 or PKCS#8) and bit
// size as metadata.
func validateChefClientKey(value []byte) ValidationResult {
	block, _ := pem.Decode(value)
	if block == nil {
		return ValidationResult{
			Valid: false,
			Error: fmt.Errorf("%w: no PEM block found", ErrInvalidPEMKey),
		}
	}

	var (
		rsaKey    *rsa.PrivateKey
		keyFormat string
	)

	switch block.Type {
	case "RSA PRIVATE KEY":
		// PKCS#1 format
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return ValidationResult{
				Valid: false,
				Error: fmt.Errorf("%w: failed to parse PKCS#1 key: %v", ErrInvalidPEMKey, err),
			}
		}
		rsaKey = key
		keyFormat = "pkcs1"

	case "PRIVATE KEY":
		// PKCS#8 format
		parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return ValidationResult{
				Valid: false,
				Error: fmt.Errorf("%w: failed to parse PKCS#8 key: %v", ErrInvalidPEMKey, err),
			}
		}
		key, ok := parsedKey.(*rsa.PrivateKey)
		if !ok {
			return ValidationResult{
				Valid: false,
				Error: fmt.Errorf("%w: PKCS#8 key is not RSA (got %T)", ErrInvalidPEMKey, parsedKey),
			}
		}
		rsaKey = key
		keyFormat = "pkcs8"

	default:
		return ValidationResult{
			Valid: false,
			Error: fmt.Errorf("%w: unexpected PEM block type %q (expected \"RSA PRIVATE KEY\" or \"PRIVATE KEY\")", ErrInvalidPEMKey, block.Type),
		}
	}

	// Validate the key structure.
	if err := rsaKey.Validate(); err != nil {
		return ValidationResult{
			Valid: false,
			Error: fmt.Errorf("%w: key validation failed: %v", ErrInvalidPEMKey, err),
		}
	}

	bits := rsaKey.N.BitLen()

	return ValidationResult{
		Valid: true,
		Metadata: map[string]any{
			"key_format": keyFormat,
			"bits":       bits,
		},
	}
}

// validateWebhookURL validates that the value is a well-formed HTTP or
// HTTPS URL.
func validateWebhookURL(value []byte) ValidationResult {
	raw := strings.TrimSpace(string(value))
	if raw == "" {
		return ValidationResult{
			Valid: false,
			Error: ErrEmptyValue,
		}
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return ValidationResult{
			Valid: false,
			Error: fmt.Errorf("%w: %v", ErrInvalidWebhookURL, err),
		}
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ValidationResult{
			Valid: false,
			Error: fmt.Errorf("%w: scheme must be http or https, got %q", ErrInvalidWebhookURL, parsed.Scheme),
		}
	}

	if parsed.Host == "" {
		return ValidationResult{
			Valid: false,
			Error: fmt.Errorf("%w: URL must include a host", ErrInvalidWebhookURL),
		}
	}

	return ValidationResult{
		Valid: true,
	}
}

// validateNonEmpty is the fallback validator for credential types that
// only require a non-empty value (ldap_bind_password, smtp_password,
// generic). The empty check is already performed by ValidateCredentialValue
// before dispatching, so this always succeeds.
func validateNonEmpty(_ []byte) ValidationResult {
	return ValidationResult{
		Valid: true,
	}
}
