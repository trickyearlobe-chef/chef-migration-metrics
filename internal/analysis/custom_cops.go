// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// compiledCustomCop holds a custom cop definition with its pre-compiled pattern.
type compiledCustomCop struct {
	def     datastore.CustomCopDefinition
	re      *regexp.Regexp // non-nil for regex patterns
	literal string         // non-empty for literal patterns
}

// ScanCustomCops runs custom cop pattern matching against cookbook source files
// in cookbookDir. Each enabled definition is matched line-by-line against files
// matching its file_glob. Returns offenses in the standard CookstyleOffense format.
func ScanCustomCops(cookbookDir string, defs []datastore.CustomCopDefinition) []CookstyleOffense {
	if len(defs) == 0 {
		return nil
	}

	compiled := make([]compiledCustomCop, 0, len(defs))
	for _, d := range defs {
		cc := compiledCustomCop{def: d}
		if d.PatternType == "regex" {
			re, err := regexp.Compile(d.Pattern)
			if err != nil {
				continue // skip invalid regex
			}
			cc.re = re
		} else {
			cc.literal = d.Pattern
		}
		compiled = append(compiled, cc)
	}

	if len(compiled) == 0 {
		return nil
	}

	var allOffenses []CookstyleOffense

	for _, cc := range compiled {
		glob := cc.def.FileGlob
		if glob == "" {
			glob = "*.rb"
		}

		_ = filepath.Walk(cookbookDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			relPath, relErr := filepath.Rel(cookbookDir, path)
			if relErr != nil {
				return nil
			}

			if !fileMatchesGlob(relPath, glob) {
				return nil
			}

			offenses := scanFileForCustomCop(path, relPath, cc)
			allOffenses = append(allOffenses, offenses...)
			return nil
		})
	}

	return allOffenses
}

// fileMatchesGlob checks whether a relative file path matches a glob pattern.
// If the glob contains no path separator, it matches against the base filename.
// Otherwise it matches against the full relative path.
func fileMatchesGlob(relPath, glob string) bool {
	if !strings.Contains(glob, "/") && !strings.Contains(glob, string(os.PathSeparator)) {
		matched, _ := filepath.Match(glob, filepath.Base(relPath))
		return matched
	}
	matched, _ := filepath.Match(glob, relPath)
	return matched
}

// scanFileForCustomCop reads a file line-by-line and produces offenses for
// lines matching the custom cop's pattern.
func scanFileForCustomCop(path, relPath string, cc compiledCustomCop) []CookstyleOffense {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var offenses []CookstyleOffense
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		var matched bool
		var matchStart, matchEnd int

		if cc.re != nil {
			loc := cc.re.FindStringIndex(line)
			if loc != nil {
				matched = true
				matchStart = loc[0] + 1 // 1-based column
				matchEnd = loc[1]
			}
		} else {
			idx := strings.Index(line, cc.literal)
			if idx >= 0 {
				matched = true
				matchStart = idx + 1 // 1-based column
				matchEnd = idx + len(cc.literal)
			}
		}

		if matched {
			offenses = append(offenses, CookstyleOffense{
				CopName:  cc.def.CopName,
				Severity: classificationToSeverity(cc.def.Classification),
				Message:  cc.def.Description,
				File:     relPath,
				Location: CookstyleOffenseLocation{
					StartLine:   lineNum,
					StartColumn: matchStart,
					LastLine:    lineNum,
					LastColumn:  matchEnd,
				},
			})
		}
	}

	return offenses
}

// classificationToSeverity maps a cop classification level to a CookStyle
// severity string for consistent offense output.
func classificationToSeverity(classification string) string {
	switch classification {
	case "blocker":
		return "error"
	case "review":
		return "warning"
	default:
		return "convention"
	}
}
