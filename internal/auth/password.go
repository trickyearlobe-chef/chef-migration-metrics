// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// DefaultBcryptCost is the bcrypt cost factor used for password hashing.
// This provides a good balance between security and performance. The value
// matches bcrypt.DefaultCost (10) which takes ~100ms on modern hardware.
const DefaultBcryptCost = bcrypt.DefaultCost

// HashPassword hashes a plaintext password using bcrypt with the default
// cost factor. The returned hash includes the algorithm, cost, salt, and
// hash and is safe to store directly in the database.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), DefaultBcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: hashing password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a plaintext password against a bcrypt hash.
// Returns nil if the password matches, or an error if it does not (or if
// the hash is malformed).
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// PasswordValidationError describes why a password failed validation.
type PasswordValidationError struct {
	Reasons []string
}

func (e *PasswordValidationError) Error() string {
	if len(e.Reasons) == 0 {
		return "password validation failed"
	}
	msg := "password validation failed:"
	for _, r := range e.Reasons {
		msg += " " + r + ";"
	}
	return msg
}

// ValidatePassword checks a plaintext password against the configured
// minimum length and basic complexity requirements. Returns nil if the
// password is acceptable, or a *PasswordValidationError listing all
// violations.
//
// Complexity rules (applied only when minLength >= 8):
//   - At least one uppercase letter
//   - At least one lowercase letter
//   - At least one digit
func ValidatePassword(password string, minLength int) error {
	if minLength <= 0 {
		minLength = 8
	}

	var reasons []string

	length := utf8.RuneCountInString(password)
	if length < minLength {
		reasons = append(reasons, fmt.Sprintf("must be at least %d characters (got %d)", minLength, length))
	}

	// Only enforce complexity when the minimum length is at least 8
	// (i.e. the default or stricter). Very short minimum lengths are
	// assumed to be intentionally relaxed (e.g. dev/test environments).
	if minLength >= 8 {
		var hasUpper, hasLower, hasDigit bool
		for _, r := range password {
			switch {
			case unicode.IsUpper(r):
				hasUpper = true
			case unicode.IsLower(r):
				hasLower = true
			case unicode.IsDigit(r):
				hasDigit = true
			}
		}
		if !hasUpper {
			reasons = append(reasons, "must contain at least one uppercase letter")
		}
		if !hasLower {
			reasons = append(reasons, "must contain at least one lowercase letter")
		}
		if !hasDigit {
			reasons = append(reasons, "must contain at least one digit")
		}
	}

	if len(reasons) > 0 {
		return &PasswordValidationError{Reasons: reasons}
	}
	return nil
}
