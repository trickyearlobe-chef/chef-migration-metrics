package chefapi

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// ---------------------------------------------------------------------------
// Role collection via the search index
// ---------------------------------------------------------------------------

// RoleSearchAttributes returns the PartialSearchQuery used to collect roles
// for the dependency graph. Only the fields BuildRoleDependencies consumes are
// requested.
//
// Roles are a top-level Chef object, not a node: their fields are not under the
// merged attribute namespace, so the paths are unprefixed. Adding an
// "automatic"/"default"/"override" segment here would silently return nothing —
// the same failure mode that emptied Windows filesystem data in v2.18.6.
func RoleSearchAttributes() PartialSearchQuery {
	return PartialSearchQuery{
		"name":          {"name"},
		"run_list":      {"run_list"},
		"env_run_lists": {"env_run_lists"},
	}
}

// CollectAllRoles collects every role in the organisation from the `role`
// search index, returning them sorted by name with duplicates removed.
//
// The sort is not cosmetic. Measured against a real Chef Infra Server (15.10),
// the role index applies no stable ordering: the same 42 roles come back in a
// different order depending on the page size requested. The set was identical
// every time, but relying on server order would make the persisted dependency
// graph differ run to run for no reason.
//
// The same measurement is why the caller's per-role gap fill is load-bearing
// rather than belt-and-braces: an index with no stable sort can shift under a
// paginated walk, and at a large role count that walk spans many requests.
//
// The Chef API has no bulk role-detail endpoint, so the alternative is one
// GET /roles/<name> per role, which at a large role count dominates the
// collection cycle. Partial search replaces that with roughly one request per
// page.
//
// Two properties of the role index are unverified at scale, so both are handled
// defensively:
//
//   - The index may enforce a lower `rows` cap than the node index. Pagination
//     therefore advances by the number of rows actually returned, never by the
//     requested page size — otherwise every role beyond the cap in each page
//     would be skipped silently.
//   - Pagination boundaries may duplicate rows, as they do for nodes (cf.
//     deduplicateSnapshotParams). Roles are de-duplicated by name, first
//     occurrence winning.
//
// A page that returns no rows ends the walk rather than failing: a short read is
// not evidence of an error, and the caller fills any gap with the per-role GET
// fallback.
func (c *Client) CollectAllRoles(ctx context.Context, pageSize, concurrency int) ([]*RoleDetail, error) {
	if pageSize <= 0 {
		pageSize = 1000
	}
	if concurrency <= 0 {
		concurrency = 1
	}

	attrs := RoleSearchAttributes()

	// First page — discover the total and the page size the server actually
	// honours.
	first, err := c.PartialSearch(ctx, "role", "*:*", pageSize, 0, attrs)
	if err != nil {
		return nil, fmt.Errorf("chefapi: first page of role search: %w", err)
	}

	roles := make([]*RoleDetail, 0, first.Total)
	seen := make(map[string]struct{}, first.Total)
	appendRows := func(rows []SearchResultRow) {
		for _, row := range rows {
			role, ok := decodeRoleRow(row)
			if !ok {
				continue
			}
			if _, dup := seen[role.Name]; dup {
				continue
			}
			seen[role.Name] = struct{}{}
			roles = append(roles, role)
		}
	}

	appendRows(first.Rows)

	// The effective page size is what the server returned, not what was asked
	// for. Zero means there is nothing to walk.
	effective := len(first.Rows)
	if effective == 0 {
		return roles, nil
	}

	// Walk the remaining offsets in waves of `concurrency` requests. Offsets
	// within a wave are known in advance (they are multiples of the effective
	// page size); the next wave's starting offset is not, because a page may
	// come back short.
	for next := effective; next < first.Total; {
		offsets := make([]int, 0, concurrency)
		for i := 0; i < concurrency; i++ {
			start := next + i*effective
			if start >= first.Total {
				break
			}
			offsets = append(offsets, start)
		}
		if len(offsets) == 0 {
			break
		}

		pages, err := c.fetchRolePages(ctx, offsets, pageSize, attrs)
		if err != nil {
			return nil, err
		}

		// Assemble in offset order so the result is deterministic regardless
		// of completion order.
		fetched := 0
		short := false
		for _, start := range offsets {
			rows := pages[start]
			appendRows(rows)
			fetched += len(rows)
			if len(rows) == 0 {
				// An empty page means the index has no more rows to give,
				// whatever `total` claimed. Stop rather than spin.
				short = true
				break
			}
		}
		if short {
			break
		}
		next += fetched
	}

	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })

	return roles, nil
}

// fetchRolePages fetches the given page offsets concurrently, returning the
// rows keyed by offset. The first error encountered is returned; partial
// results are discarded because a missing page would silently truncate the
// dependency graph.
func (c *Client) fetchRolePages(ctx context.Context, offsets []int, pageSize int, attrs PartialSearchQuery) (map[int][]SearchResultRow, error) {
	var (
		mu       sync.Mutex
		pages    = make(map[int][]SearchResultRow, len(offsets))
		firstErr error
		wg       sync.WaitGroup
	)

	for _, start := range offsets {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()

			if ctx.Err() != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("chefapi: role search page at start=%d: %w", start, ctx.Err())
				}
				mu.Unlock()
				return
			}

			page, err := c.PartialSearch(ctx, "role", "*:*", pageSize, start, attrs)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("chefapi: role search page at start=%d: %w", start, err)
				}
				return
			}
			pages[start] = page.Rows
		}(start)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return pages, nil
}

// decodeRoleRow converts one search row into a RoleDetail. Chef object shapes
// vary across versions, so a row that cannot be understood is dropped rather
// than failing the collection: a partial graph is more useful than none, and
// matches the behaviour of the per-role fetch path. A row with no usable name
// is unusable — everything downstream keys on it.
func decodeRoleRow(row SearchResultRow) (*RoleDetail, bool) {
	if row.Data == nil {
		return nil, false
	}

	name, ok := row.Data["name"].(string)
	if !ok || name == "" {
		return nil, false
	}

	return &RoleDetail{
		Name:        name,
		RunList:     decodeStringList(row.Data["run_list"]),
		EnvRunLists: decodeEnvRunLists(row.Data["env_run_lists"]),
	}, true
}

// decodeStringList returns the string members of a JSON array, dropping
// non-string entries individually rather than discarding the whole list. A
// value that is not an array at all yields nil.
func decodeStringList(v interface{}) []string {
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		if s, ok := entry.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// decodeEnvRunLists returns the per-environment run lists. Environments whose
// value is not an array are dropped, so a malformed entry cannot masquerade as
// an environment with an empty run list.
func decodeEnvRunLists(v interface{}) map[string][]string {
	raw, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string][]string, len(raw))
	for env, entries := range raw {
		if _, isList := entries.([]interface{}); !isList {
			continue
		}
		out[env] = decodeStringList(entries)
	}
	return out
}
