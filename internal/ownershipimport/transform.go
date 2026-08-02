// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Transform is one step in a field's transform chain. Every transform is
// strictly text in, text out; none reads a column. That separation from Source
// is what keeps a mapping document readable — the source answers "which cells",
// the transforms answer "what is done to the text".
type Transform struct {
	Kind string `json:"kind"`

	// Value carries the operand for prefix, suffix and default.
	Value string `json:"value,omitempty"`

	// From and To carry the operands for replace.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`

	// Pattern carries the expression for regex_extract.
	Pattern string `json:"pattern,omitempty"`
}

// Transform kinds. The catalogue is closed: an unknown kind is a mapping
// validation error, never a silently skipped step.
const (
	TransformTrim         = "trim"
	TransformLowercase    = "lowercase"
	TransformUppercase    = "uppercase"
	TransformStripDomain  = "strip_domain"
	TransformPrefix       = "prefix"
	TransformSuffix       = "suffix"
	TransformReplace      = "replace"
	TransformRegexExtract = "regex_extract"
	// TransformDefault is deliberately named "default" rather than "constant",
	// so it cannot be confused with the constant *source*.
	TransformDefault = "default"
)

// CompiledChain is a transform list with its patterns already compiled. It
// holds no per-row state, so one chain serves every row of an import.
type CompiledChain []func(string) string

// CompileTransforms validates a transform list and prepares it for use.
// Compiling up front is what turns an uncompilable pattern into a mapping error
// the administrator sees once, rather than the same per-row failure repeated
// for every line of the file.
func CompileTransforms(transforms []Transform) (CompiledChain, error) {
	chain := make(CompiledChain, 0, len(transforms))

	for i, t := range transforms {
		fn, err := compileTransform(t)
		if err != nil {
			return nil, fmt.Errorf("transforms[%d]: %w", i, err)
		}
		chain = append(chain, fn)
	}
	return chain, nil
}

func compileTransform(t Transform) (func(string) string, error) {
	switch t.Kind {
	case TransformTrim:
		return strings.TrimSpace, nil

	case TransformLowercase:
		return strings.ToLower, nil

	case TransformUppercase:
		return strings.ToUpper, nil

	case TransformStripDomain:
		return stripDomain, nil

	case TransformPrefix:
		return func(s string) string { return t.Value + s }, nil

	case TransformSuffix:
		return func(s string) string { return s + t.Value }, nil

	case TransformReplace:
		if t.From == "" {
			return nil, fmt.Errorf("replace needs a %q to substitute", "from")
		}
		return func(s string) string { return strings.ReplaceAll(s, t.From, t.To) }, nil

	case TransformRegexExtract:
		if t.Pattern == "" {
			return nil, fmt.Errorf("regex_extract needs a %q", "pattern")
		}
		re, err := regexp.Compile(t.Pattern)
		if err != nil {
			return nil, fmt.Errorf("regex_extract pattern %q will not compile: %w", t.Pattern, err)
		}
		return func(s string) string { return firstCaptureGroup(re, s) }, nil

	case TransformDefault:
		return func(s string) string {
			if s == "" {
				return t.Value
			}
			return s
		}, nil

	case "":
		return nil, fmt.Errorf("a transform needs a %q", "kind")

	default:
		return nil, fmt.Errorf("unknown transform %q", t.Kind)
	}
}

// Apply runs the chain left to right.
func (c CompiledChain) Apply(s string) string {
	for _, fn := range c {
		s = fn(s)
	}
	return s
}

// stripDomain takes the part before the first "@".
//
// An address literal is left unchanged. A host part that is an address, not a
// name, has no domain to strip, and truncating it produces a value that looks
// plausible and is wrong — which is the worst kind of import defect, because
// nothing downstream can detect it.
func stripDomain(s string) string {
	if net.ParseIP(s) != nil {
		return s
	}
	if at := strings.Index(s, "@"); at >= 0 {
		return s[:at]
	}
	return s
}

// firstCaptureGroup yields the first capture group of the first match, or the
// empty string when the pattern does not match or captures nothing.
//
// It never passes the input through unchanged. A silent pass-through would let
// unextracted raw text reach an owner name and create owners nobody
// recognises, with the mapping looking as though it had worked.
func firstCaptureGroup(re *regexp.Regexp, s string) string {
	match := re.FindStringSubmatch(s)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
