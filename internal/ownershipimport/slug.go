// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import (
	"fmt"
	"regexp"
	"strings"
)

// OwnerNameRe mirrors the CHECK constraint on owners.name:
//
//	CONSTRAINT owners_name_format CHECK (name ~ '^[a-z0-9][a-z0-9._-]*$')
//
// migrations/0001_initial_schema.up.sql:729. Keeping the two in step is asserted
// by a property test, not by inspection.
var OwnerNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// leadingPunctuation is the set the constraint forbids in first position but
// permits thereafter.
const leadingPunctuation = "-._"

// SlugifyOwnerName turns an arbitrary source string into a legal owner name.
//
// It is applied implicitly and unconditionally as the final step of the owner
// field, after that field's own transform chain. It is deliberately not a
// transform: making it one would let a mapping omit it and produce a document
// that validates and then fails at write time against a database constraint.
//
// Accents are not stripped. Unicode decomposition would need a module in
// neither go.mod nor go.sum, for one cosmetic transformation, and every new
// dependency carries a supply-chain check. The accepted trade-off is that an
// accented value folds toward hyphens rather than to its ASCII base
// ("Renée" becomes "ren-e"). This costs nothing in matching, because the raw
// string is preserved as display_name and seeded as a custom alias — fuzzy
// matching and every future import compare against the original. The slug is
// only a stable, constraint-legal handle.
func SlugifyOwnerName(raw string) (string, error) {
	var b strings.Builder
	b.Grow(len(raw))

	for _, r := range strings.ToLower(raw) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			// Every rune outside the permitted set becomes a single hyphen.
			// Non-ASCII runes are not decomposed.
			b.WriteRune('-')
		}
	}

	slug := collapseHyphens(b.String())
	slug = strings.TrimLeft(slug, leadingPunctuation)
	slug = strings.TrimRight(slug, "-")

	if slug == "" {
		return "", fmt.Errorf("ownershipimport: %q cannot become an owner name — nothing legal remains after normalisation", raw)
	}
	if !OwnerNameRe.MatchString(slug) {
		return "", fmt.Errorf("ownershipimport: %q normalises to %q, which is not a legal owner name", raw, slug)
	}
	return slug, nil
}

func collapseHyphens(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	var prevHyphen bool
	for _, r := range s {
		if r == '-' && prevHyphen {
			continue
		}
		prevHyphen = r == '-'
		b.WriteRune(r)
	}
	return b.String()
}
