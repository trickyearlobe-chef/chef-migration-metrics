// SPDX-License-Identifier: Apache-2.0

package gitkitchen

// ComputeTKStatus returns the aggregate Test Kitchen status from pass/fail counts.
// All Go-side TK status derivation must use this function to stay consistent.
//
//   - passed > 0 && failed > 0 → "partial"
//   - failed > 0               → "failed"
//   - passed > 0               → "passed"
//   - otherwise                → "" (no data)
func ComputeTKStatus(passed, failed int) string {
	switch {
	case passed > 0 && failed > 0:
		return "partial"
	case failed > 0:
		return "failed"
	case passed > 0:
		return "passed"
	default:
		return ""
	}
}
