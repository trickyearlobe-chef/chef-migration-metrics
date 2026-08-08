// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// ImportMapping is a saved column mapping for the discovery-driven ownership
// import. It exists so a repeat import needs no re-mapping.
//
// FieldMap is stored and returned as raw JSON: it is a nested tagged union
// owned by internal/ownershipimport, and the datastore has no business
// interpreting it. Shredding it into columns would turn every change to the
// mapping language into a migration.
type ImportMapping struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	SourceKind string          `json:"source_kind"`
	Delimiter  string          `json:"delimiter"`
	FieldMap   json.RawMessage `json:"field_map,omitempty"`
	CreatedBy  string          `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`

	// Where a database import reads from. DBCredential names a stored
	// credential; the connection string itself is never held here, so this
	// table never becomes the one place a password sits in plain text.
	DBDriver     string `json:"db_driver,omitempty"`
	DBCredential string `json:"db_credential,omitempty"`
	DBQuery      string `json:"db_query,omitempty"`

	// Import options that change what a row does, saved because an unattended
	// run must repeat what the person previewed rather than today's defaults.
	FilterColumn string `json:"filter_column,omitempty"`
	FilterValue  string `json:"filter_value,omitempty"`
	CreateOwners bool   `json:"create_owners"`

	// Standard 5-field cron. Empty means unscheduled.
	Schedule        string `json:"schedule"`
	ScheduleEnabled bool   `json:"schedule_enabled"`

	// What the last unattended run did, so a silently failing import is
	// visible from the list without reading the audit log.
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastRunStatus string     `json:"last_run_status,omitempty"`
	LastRunDetail string     `json:"last_run_detail,omitempty"`
}

// InsertImportMappingParams holds the fields for creating a saved mapping.
type InsertImportMappingParams struct {
	Name       string
	SourceKind string
	Delimiter  string
	FieldMap   json.RawMessage
	CreatedBy  string

	ImportMappingSource
}

// ImportMappingSource is everything a saved import needs in order to run
// without a person present: what to read, which rows to keep, and when.
//
// Grouped into one type because create and update take exactly the same set —
// a saved import that could be created scheduled but not rescheduled, or the
// reverse, is the kind of asymmetry that goes unnoticed until somebody cannot
// turn one off.
type ImportMappingSource struct {
	DBDriver     string
	DBCredential string
	DBQuery      string

	FilterColumn string
	FilterValue  string
	CreateOwners bool

	Schedule        string
	ScheduleEnabled bool
}

// UpdateImportMappingParams holds the replaceable fields of a saved mapping.
// Editing a mapping never re-runs a past import — a mapping is a template, not
// a record of what happened.
type UpdateImportMappingParams struct {
	Name      string
	Delimiter string
	FieldMap  json.RawMessage

	ImportMappingSource
}

// importMappingSourceColumns are the fields a saved import needs to run
// unattended, plus what its last run did.
const importMappingSourceColumns = `db_driver, db_credential, db_query, ` +
	`filter_column, filter_value, create_owners, schedule, schedule_enabled, ` +
	`last_run_at, last_run_status, last_run_detail`

const importMappingColumns = `id, name, source_kind, delimiter, field_map, created_by, created_at, updated_at, ` +
	importMappingSourceColumns

// importMappingSummaryColumns omits field_map. The list endpoint returns
// identity and provenance only: a page of twenty mapping documents is a lot of
// JSON nobody on that screen reads.
//
// The schedule and the last run stay in the summary — they are exactly what
// somebody opens the list to check, and fetching each import in turn to find
// out whether it is running is not a list.
const importMappingSummaryColumns = `id, name, source_kind, delimiter, created_by, created_at, updated_at, ` +
	importMappingSourceColumns

// importMappingSourceDests are the scan targets for importMappingSourceColumns,
// in the same order. Shared by both scanners so the two column lists cannot
// drift apart from the two Scan calls.
func importMappingSourceDests(m *ImportMapping, lastRunAt *sql.NullTime) []any {
	return []any{
		&m.DBDriver, &m.DBCredential, &m.DBQuery,
		&m.FilterColumn, &m.FilterValue, &m.CreateOwners,
		&m.Schedule, &m.ScheduleEnabled,
		lastRunAt, &m.LastRunStatus, &m.LastRunDetail,
	}
}

func scanImportMapping(row interface{ Scan(dest ...any) error }) (ImportMapping, error) {
	var (
		m         ImportMapping
		fieldMap  []byte
		createdBy sql.NullString
		lastRunAt sql.NullTime
	)
	dest := append([]any{&m.ID, &m.Name, &m.SourceKind, &m.Delimiter, &fieldMap, &createdBy, &m.CreatedAt, &m.UpdatedAt},
		importMappingSourceDests(&m, &lastRunAt)...)
	if err := row.Scan(dest...); err != nil {
		return ImportMapping{}, err
	}
	m.FieldMap = json.RawMessage(fieldMap)
	m.CreatedBy = createdBy.String
	if lastRunAt.Valid {
		t := lastRunAt.Time
		m.LastRunAt = &t
	}
	return m, nil
}

func scanImportMappingSummary(row interface{ Scan(dest ...any) error }) (ImportMapping, error) {
	var (
		m         ImportMapping
		createdBy sql.NullString
		lastRunAt sql.NullTime
	)
	dest := append([]any{&m.ID, &m.Name, &m.SourceKind, &m.Delimiter, &createdBy, &m.CreatedAt, &m.UpdatedAt},
		importMappingSourceDests(&m, &lastRunAt)...)
	if err := row.Scan(dest...); err != nil {
		return ImportMapping{}, err
	}
	m.CreatedBy = createdBy.String
	if lastRunAt.Valid {
		t := lastRunAt.Time
		m.LastRunAt = &t
	}
	return m, nil
}

// InsertImportMapping creates a saved mapping. Returns ErrAlreadyExists when
// the name is taken.
func (db *DB) InsertImportMapping(ctx context.Context, p InsertImportMappingParams) (ImportMapping, error) {
	if p.Name == "" {
		return ImportMapping{}, errors.New("datastore: name is required")
	}
	if len(p.FieldMap) == 0 {
		return ImportMapping{}, errors.New("datastore: field_map is required")
	}

	sourceKind := p.SourceKind
	if sourceKind == "" {
		sourceKind = "csv"
	}
	delimiter := p.Delimiter
	if delimiter == "" {
		delimiter = ","
	}

	query := `
		INSERT INTO ownership_import_mappings (
			name, source_kind, delimiter, field_map, created_by,
			db_driver, db_credential, db_query,
			filter_column, filter_value, create_owners,
			schedule, schedule_enabled)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING ` + importMappingColumns

	row := db.pool.QueryRowContext(ctx, query,
		p.Name, sourceKind, delimiter, []byte(p.FieldMap), p.CreatedBy,
		p.DBDriver, p.DBCredential, p.DBQuery,
		p.FilterColumn, p.FilterValue, p.CreateOwners,
		p.Schedule, p.ScheduleEnabled)
	m, err := scanImportMapping(row)
	if err != nil {
		if isUniqueViolation(err) {
			return ImportMapping{}, ErrAlreadyExists
		}
		return ImportMapping{}, fmt.Errorf("datastore: inserting import mapping: %w", err)
	}
	return m, nil
}

// ListImportMappings returns saved mappings ordered by name, without their
// field maps, alongside the total count for pagination.
func (db *DB) ListImportMappings(ctx context.Context, limit, offset int) ([]ImportMapping, int, error) {
	var total int
	if err := db.pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM ownership_import_mappings`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("datastore: counting import mappings: %w", err)
	}

	query := `SELECT ` + importMappingSummaryColumns + `
		FROM ownership_import_mappings
		ORDER BY name
		LIMIT $1 OFFSET $2`

	rows, err := db.pool.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: querying import mappings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var mappings []ImportMapping
	for rows.Next() {
		m, scanErr := scanImportMappingSummary(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("datastore: scanning import mapping: %w", scanErr)
		}
		mappings = append(mappings, m)
	}
	return mappings, total, rows.Err()
}

// GetImportMapping returns one saved mapping including its field map. Returns
// ErrNotFound when no mapping has that id.
func (db *DB) GetImportMapping(ctx context.Context, id int64) (ImportMapping, error) {
	query := `SELECT ` + importMappingColumns + ` FROM ownership_import_mappings WHERE id = $1`

	m, err := scanImportMapping(db.pool.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ImportMapping{}, ErrNotFound
	}
	if err != nil {
		return ImportMapping{}, fmt.Errorf("datastore: getting import mapping %d: %w", id, err)
	}
	return m, nil
}

// UpdateImportMapping replaces a saved mapping's name, delimiter and field map.
// Returns ErrNotFound when no mapping has that id, and ErrAlreadyExists when
// the new name is taken by another mapping.
func (db *DB) UpdateImportMapping(ctx context.Context, id int64, p UpdateImportMappingParams) (ImportMapping, error) {
	if p.Name == "" {
		return ImportMapping{}, errors.New("datastore: name is required")
	}
	if len(p.FieldMap) == 0 {
		return ImportMapping{}, errors.New("datastore: field_map is required")
	}

	delimiter := p.Delimiter
	if delimiter == "" {
		delimiter = ","
	}

	query := `
		UPDATE ownership_import_mappings
		SET name = $2, delimiter = $3, field_map = $4,
		    db_driver = $5, db_credential = $6, db_query = $7,
		    filter_column = $8, filter_value = $9, create_owners = $10,
		    schedule = $11, schedule_enabled = $12,
		    updated_at = now()
		WHERE id = $1
		RETURNING ` + importMappingColumns

	m, err := scanImportMapping(db.pool.QueryRowContext(ctx, query,
		id, p.Name, delimiter, []byte(p.FieldMap),
		p.DBDriver, p.DBCredential, p.DBQuery,
		p.FilterColumn, p.FilterValue, p.CreateOwners,
		p.Schedule, p.ScheduleEnabled))
	if errors.Is(err, sql.ErrNoRows) {
		return ImportMapping{}, ErrNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return ImportMapping{}, ErrAlreadyExists
		}
		return ImportMapping{}, fmt.Errorf("datastore: updating import mapping %d: %w", id, err)
	}
	return m, nil
}

// DeleteImportMapping removes a saved mapping. Returns ErrNotFound when no
// mapping has that id. Nothing references the table, so there is no cascade.
func (db *DB) DeleteImportMapping(ctx context.Context, id int64) error {
	result, err := db.pool.ExecContext(ctx, `DELETE FROM ownership_import_mappings WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting import mapping %d: %w", id, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("datastore: deleting import mapping %d: %w", id, err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListScheduledImports returns every saved import with its schedule switched
// on, including its field map — the scheduler needs the whole definition in
// order to run one, and there are only ever a handful.
//
// Read fresh on each tick rather than cached at startup, so a schedule somebody
// changes in the UI takes effect without a restart.
func (db *DB) ListScheduledImports(ctx context.Context) ([]ImportMapping, error) {
	query := `SELECT ` + importMappingColumns + `
		FROM ownership_import_mappings
		WHERE schedule_enabled
		ORDER BY id`

	rows, err := db.pool.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying scheduled imports: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var mappings []ImportMapping
	for rows.Next() {
		m, scanErr := scanImportMapping(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("datastore: scanning scheduled import: %w", scanErr)
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// RecordImportRun stores the outcome of an unattended run.
//
// Deliberately not an error the caller has to handle by aborting: the import
// itself has already happened by the time this is written, so failing to record
// it must not be reported as the import failing.
func (db *DB) RecordImportRun(ctx context.Context, id int64, status, detail string) error {
	_, err := db.pool.ExecContext(ctx, `
		UPDATE ownership_import_mappings
		SET last_run_at = now(), last_run_status = $2, last_run_detail = $3
		WHERE id = $1`, id, status, detail)
	if err != nil {
		return fmt.Errorf("datastore: recording import run %d: %w", id, err)
	}
	return nil
}

// EntityAssignment is one existing ownership assignment on an entity, carrying
// the organisation because the uniqueness rule is organisation-scoped: the same
// owner and entity in two organisations are two distinct assignments.
type EntityAssignment struct {
	OwnerName        string `json:"owner_name"`
	OrganisationName string `json:"organisation_name"`
}

// LookupAssignmentOwnersByEntity returns, for each requested entity key, the
// assignments that already exist on it. Keys with no assignment are absent from
// the map rather than present and empty.
//
// It is one query for the whole batch. The import classifier needs this for
// every row, and a per-row query would make a ten-thousand-row preview ten
// thousand round trips.
func (db *DB) LookupAssignmentOwnersByEntity(ctx context.Context, entityType string, entityKeys []string) (map[string][]EntityAssignment, error) {
	out := make(map[string][]EntityAssignment)
	if entityType == "" || len(entityKeys) == 0 {
		return out, nil
	}

	query := `
		SELECT entity_key, owner_name, COALESCE(organisation_name, '')
		FROM ownership_assignments
		WHERE entity_type = $1 AND entity_key = ANY($2)`

	rows, err := db.pool.QueryContext(ctx, query, entityType, pq.Array(entityKeys))
	if err != nil {
		return nil, fmt.Errorf("datastore: looking up assignments for %s entities: %w", entityType, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			key string
			a   EntityAssignment
		)
		if err := rows.Scan(&key, &a.OwnerName, &a.OrganisationName); err != nil {
			return nil, fmt.Errorf("datastore: scanning assignment for %s entity: %w", entityType, err)
		}
		out[key] = append(out[key], a)
	}
	return out, rows.Err()
}

// SuggestOwnersByEmailLocalpart finds owners whose email-shaped aliases share a
// localpart with the given one — "alice@corp.example" and
// "alice@users.noreply.example" being the same person under two domains, which
// is the normal shape of git commit history.
//
// It returns suggestions, never a match. The same localpart under two domains
// is just as often two different people, and the committer-assign path already
// forces owner names to the localpart — so treating this as authoritative is
// precisely how one person inherits another's identity. A human confirms.
func (db *DB) SuggestOwnersByEmailLocalpart(ctx context.Context, localpart string, limit int) ([]AliasSuggestion, error) {
	if localpart == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT DISTINCT ON (owner_name) owner_name, alias_value, alias_type
		FROM owner_aliases
		WHERE alias_type IN ('email', 'git_email')
		  AND lower(split_part(alias_value, '@', 1)) = lower($1)
		ORDER BY owner_name
		LIMIT $2`

	rows, err := db.pool.QueryContext(ctx, query, localpart, limit)
	if err != nil {
		return nil, fmt.Errorf("datastore: suggesting owners by email localpart: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AliasSuggestion
	for rows.Next() {
		var s AliasSuggestion
		if err := rows.Scan(&s.OwnerName, &s.AliasValue, &s.AliasType); err != nil {
			return nil, fmt.Errorf("datastore: scanning localpart suggestion: %w", err)
		}
		// An exact localpart hit is a stronger signal than any trigram score,
		// so it is reported as certainty about the localpart — not about the
		// person, which is what still needs confirming.
		s.Similarity = 1
		out = append(out, s)
	}
	return out, rows.Err()
}

// entityExistenceQueries maps an assignment entity type to the query that finds
// which of a set of keys CMM has actually collected.
//
// There is deliberately no entry for "policy": CMM collects no policy objects,
// so a policy key can never be confirmed. That reports as not collected, which
// is honest and — because a not-found entity never rejects a row — costs the
// import nothing.
var entityExistenceQueries = map[string]string{
	"node":     `SELECT DISTINCT node_name FROM node_snapshots WHERE node_name = ANY($1)`,
	"cookbook": `SELECT DISTINCT name FROM server_cookbooks WHERE name = ANY($1)`,
	"git_repo": `SELECT DISTINCT name FROM git_repos WHERE name = ANY($1)`,
	"role":     `SELECT DISTINCT role_name FROM role_dependencies WHERE role_name = ANY($1)`,
}

// EntityKeysExist reports which of the given keys name an entity CMM has
// collected.
//
// This is informational only. Ownership assignments are soft references with no
// foreign key, and assigning ownership before collection has run is a primary
// use case — so an absent key is reported, never rejected.
func (db *DB) EntityKeysExist(ctx context.Context, entityType string, keys []string) (map[string]bool, error) {
	out := make(map[string]bool)
	if len(keys) == 0 {
		return out, nil
	}

	query, ok := entityExistenceQueries[entityType]
	if !ok {
		return out, nil
	}

	rows, err := db.pool.QueryContext(ctx, query, pq.Array(keys))
	if err != nil {
		return nil, fmt.Errorf("datastore: checking %s entity keys: %w", entityType, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("datastore: scanning %s entity key: %w", entityType, err)
		}
		out[key] = true
	}
	return out, rows.Err()
}
