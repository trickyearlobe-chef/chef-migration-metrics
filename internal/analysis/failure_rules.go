package analysis

import (
	"sort"
	"strings"
)

// CookstyleFailureRules holds the resolved failure rules: a map of cop
// namespace prefixes to severity lists, plus a pre-sorted prefix slice for
// longest-prefix-first matching.
type CookstyleFailureRules struct {
	Rules          map[string][]string
	sortedPrefixes []string // longest first, excludes "*"
}

// NewCookstyleFailureRules builds a CookstyleFailureRules from a raw map,
// pre-sorting the prefixes by length descending for longest-prefix matching.
func NewCookstyleFailureRules(rules map[string][]string) CookstyleFailureRules {
	prefixes := make([]string, 0, len(rules))
	for k := range rules {
		if k != "*" {
			prefixes = append(prefixes, k)
		}
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})
	return CookstyleFailureRules{Rules: rules, sortedPrefixes: prefixes}
}

// DefaultFailureRules returns the "default" preset: fail on error or fatal
// regardless of namespace. This matches the legacy isErrorOrFatal behaviour.
func DefaultFailureRules() CookstyleFailureRules {
	return NewCookstyleFailureRules(map[string][]string{
		"*": {"error", "fatal"},
	})
}

// StrictFailureRules returns the "strict" preset: additionally fails on
// warnings in Deprecations/Correctness namespaces.
func StrictFailureRules() CookstyleFailureRules {
	return NewCookstyleFailureRules(map[string][]string{
		"Chef/Deprecations/": {"warning", "error", "fatal"},
		"Chef/Correctness/":  {"warning", "error", "fatal"},
		"*":                  {"error", "fatal"},
	})
}

// RelaxedFailureRules returns the "relaxed" preset: only Deprecations and
// Correctness errors cause failure; Style/Modernize/other cops never fail.
func RelaxedFailureRules() CookstyleFailureRules {
	return NewCookstyleFailureRules(map[string][]string{
		"Chef/Deprecations/": {"error", "fatal"},
		"Chef/Correctness/":  {"error", "fatal"},
		"Chef/Style/":        {},
		"Chef/Modernize/":    {},
		"*":                  {},
	})
}

// EffectiveRules resolves the active failure rules from a preset name and
// optional explicit overrides. When explicit is non-nil and non-empty, it
// takes precedence over the preset.
func EffectiveRules(preset string, explicit map[string][]string) CookstyleFailureRules {
	if len(explicit) > 0 {
		return NewCookstyleFailureRules(explicit)
	}
	switch preset {
	case "strict":
		return StrictFailureRules()
	case "relaxed":
		return RelaxedFailureRules()
	default:
		return DefaultFailureRules()
	}
}

// EvaluatePassFail evaluates whether a set of offenses passes the given
// failure rules. Returns true if passed (no offense triggered failure).
func EvaluatePassFail(offenses []CookstyleOffense, rules CookstyleFailureRules) bool {
	for i := range offenses {
		if offenseTriggersFailure(&offenses[i], &rules) {
			return false
		}
	}
	return true
}

// offenseTriggersFailure checks whether a single offense triggers failure
// under the given rules.
func offenseTriggersFailure(off *CookstyleOffense, rules *CookstyleFailureRules) bool {
	severities := matchRule(off.CopName, rules)
	if severities == nil {
		return false
	}
	for _, s := range severities {
		if s == off.Severity {
			return true
		}
	}
	return false
}

// matchRule finds the severity list for a cop name using longest-prefix match.
func matchRule(copName string, rules *CookstyleFailureRules) []string {
	for _, prefix := range rules.sortedPrefixes {
		if strings.HasPrefix(copName, prefix) {
			return rules.Rules[prefix]
		}
	}
	if catchAll, ok := rules.Rules["*"]; ok {
		return catchAll
	}
	return nil
}
