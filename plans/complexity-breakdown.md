# Complexity Score Clarity & TK Alignment

## Goal

Make it obvious that the "Critical (63)" badge on remediation pages is a weighted complexity score, not a raw offense count. Show the formula breakdown. Align TK in the complexity scorer with the actual tkstatus model.

## Steps

1. Replace stale TestKitchenSummary with TKStatus aligned to tkstatus package
2. Wire TK status into git repo scoring so it actually contributes
3. Expose complexity_breakdown in remediation API responses
4. Add ComplexityBreakdownDisplay component to frontend
5. Evaluate DB migration (not needed — TK derived at request time)

## Acceptance Criteria

- Remediation pages show formula breakdown (only non-zero components)
- TK results contribute to complexity scores (failed=20, partial=10)
- No double-counting of TK
- All tests pass
