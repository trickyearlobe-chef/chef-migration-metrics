// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword(t *testing.T) {
	password := "TestP@ss1"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword(%q) returned error: %v", password, err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty hash")
	}
	if hash == password {
		t.Fatal("HashPassword returned plaintext password instead of hash")
	}

	// The hash should be a valid bcrypt string.
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost() returned error for hash: %v", err)
	}
	if cost != DefaultBcryptCost {
		t.Errorf("bcrypt cost = %d, want %d", cost, DefaultBcryptCost)
	}
}

func TestHashPasswordDifferentSalts(t *testing.T) {
	password := "SamePassword1"
	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("first HashPassword call failed: %v", err)
	}
	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("second HashPassword call failed: %v", err)
	}
	if hash1 == hash2 {
		t.Error("two calls to HashPassword with the same password produced identical hashes — salt not applied")
	}
}

func TestCheckPasswordCorrect(t *testing.T) {
	password := "CorrectHorse1"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if err := CheckPassword(hash, password); err != nil {
		t.Errorf("CheckPassword should succeed for correct password, got: %v", err)
	}
}

func TestCheckPasswordIncorrect(t *testing.T) {
	password := "CorrectHorse1"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if err := CheckPassword(hash, "WrongPassword1"); err == nil {
		t.Error("CheckPassword should fail for incorrect password, got nil")
	}
}

func TestCheckPasswordMalformedHash(t *testing.T) {
	if err := CheckPassword("not-a-valid-hash", "anything"); err == nil {
		t.Error("CheckPassword should fail for malformed hash, got nil")
	}
}

func TestCheckPasswordEmptyHash(t *testing.T) {
	if err := CheckPassword("", "anything"); err == nil {
		t.Error("CheckPassword should fail for empty hash, got nil")
	}
}

func TestCheckPasswordEmptyPassword(t *testing.T) {
	hash, err := HashPassword("RealPassword1")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if err := CheckPassword(hash, ""); err == nil {
		t.Error("CheckPassword should fail for empty password against a real hash, got nil")
	}
}

// ---------------------------------------------------------------------------
// ValidatePassword tests
// ---------------------------------------------------------------------------

func TestValidatePasswordTooShort(t *testing.T) {
	err := ValidatePassword("Aa1", 8)
	if err == nil {
		t.Fatal("expected validation error for short password, got nil")
	}
	pve, ok := err.(*PasswordValidationError)
	if !ok {
		t.Fatalf("expected *PasswordValidationError, got %T", err)
	}
	found := false
	for _, r := range pve.Reasons {
		if containsStr(r, "at least 8 characters") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected reason about minimum length, got reasons: %v", pve.Reasons)
	}
}

func TestValidatePasswordExactMinLength(t *testing.T) {
	// Exactly 8 chars, has upper, lower, digit.
	err := ValidatePassword("Abcdefg1", 8)
	if err != nil {
		t.Errorf("expected no error for 8-char password meeting complexity, got: %v", err)
	}
}

func TestValidatePasswordNoUppercase(t *testing.T) {
	err := ValidatePassword("abcdefgh1", 8)
	if err == nil {
		t.Fatal("expected validation error for missing uppercase, got nil")
	}
	pve, ok := err.(*PasswordValidationError)
	if !ok {
		t.Fatalf("expected *PasswordValidationError, got %T", err)
	}
	found := false
	for _, r := range pve.Reasons {
		if containsStr(r, "uppercase") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected reason about uppercase, got: %v", pve.Reasons)
	}
}

func TestValidatePasswordNoLowercase(t *testing.T) {
	err := ValidatePassword("ABCDEFGH1", 8)
	if err == nil {
		t.Fatal("expected validation error for missing lowercase, got nil")
	}
	pve := err.(*PasswordValidationError)
	found := false
	for _, r := range pve.Reasons {
		if containsStr(r, "lowercase") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected reason about lowercase, got: %v", pve.Reasons)
	}
}

func TestValidatePasswordNoDigit(t *testing.T) {
	err := ValidatePassword("Abcdefghi", 8)
	if err == nil {
		t.Fatal("expected validation error for missing digit, got nil")
	}
	pve := err.(*PasswordValidationError)
	found := false
	for _, r := range pve.Reasons {
		if containsStr(r, "digit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected reason about digit, got: %v", pve.Reasons)
	}
}

func TestValidatePasswordMultipleViolations(t *testing.T) {
	// Short, no upper, no digit.
	err := ValidatePassword("abc", 8)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	pve := err.(*PasswordValidationError)
	if len(pve.Reasons) < 3 {
		t.Errorf("expected at least 3 reasons (length + uppercase + digit), got %d: %v",
			len(pve.Reasons), pve.Reasons)
	}
}

func TestValidatePasswordAcceptsValidPassword(t *testing.T) {
	validPasswords := []string{
		"Abcdefg1",
		"P@ssword123",
		"MyStr0ngPass!",
		"1aB56789",
	}
	for _, p := range validPasswords {
		if err := ValidatePassword(p, 8); err != nil {
			t.Errorf("ValidatePassword(%q, 8) should succeed, got: %v", p, err)
		}
	}
}

func TestValidatePasswordCustomMinLength(t *testing.T) {
	// With min length 12, an 8-char password should fail.
	err := ValidatePassword("Abcdefg1", 12)
	if err == nil {
		t.Fatal("expected validation error for password shorter than custom min length")
	}
	pve := err.(*PasswordValidationError)
	found := false
	for _, r := range pve.Reasons {
		if containsStr(r, "at least 12 characters") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected reason about 12-character minimum, got: %v", pve.Reasons)
	}
}

func TestValidatePasswordLowMinLengthSkipsComplexity(t *testing.T) {
	// When min length < 8, complexity rules are relaxed.
	err := ValidatePassword("abcde", 4)
	if err != nil {
		t.Errorf("expected no error for low min length with simple password, got: %v", err)
	}
}

func TestValidatePasswordZeroMinLengthDefaultsToEight(t *testing.T) {
	// min=0 should default to 8.
	err := ValidatePassword("short", 0)
	if err == nil {
		t.Fatal("expected validation error when minLength=0 defaults to 8 and password is too short")
	}
}

func TestValidatePasswordNegativeMinLengthDefaultsToEight(t *testing.T) {
	err := ValidatePassword("Abc1efgh", -1)
	if err != nil {
		t.Errorf("expected no error for 8-char valid password with negative minLength, got: %v", err)
	}
}

func TestValidatePasswordUnicodeCharacters(t *testing.T) {
	// Unicode letters count toward the length and complexity rules.
	// "Über1234" has uppercase, lowercase, digit, and 8+ rune count.
	err := ValidatePassword("Über1234", 8)
	if err != nil {
		t.Errorf("expected no error for Unicode password meeting all rules, got: %v", err)
	}
}

func TestPasswordValidationErrorMessage(t *testing.T) {
	pve := &PasswordValidationError{Reasons: []string{"too short", "no digit"}}
	msg := pve.Error()
	if msg == "" {
		t.Fatal("Error() returned empty string")
	}
	if !containsStr(msg, "too short") {
		t.Errorf("Error() message should contain 'too short', got: %s", msg)
	}
	if !containsStr(msg, "no digit") {
		t.Errorf("Error() message should contain 'no digit', got: %s", msg)
	}
}

func TestPasswordValidationErrorMessageNoReasons(t *testing.T) {
	pve := &PasswordValidationError{}
	msg := pve.Error()
	if msg == "" {
		t.Fatal("Error() returned empty string for zero-reason error")
	}
	if !containsStr(msg, "password validation failed") {
		t.Errorf("expected generic message, got: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
