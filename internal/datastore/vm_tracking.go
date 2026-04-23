// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// TrackedVM represents a row in the vm_tracking table.
type TrackedVM struct {
	ID                string    `json:"id"`
	VMName            string    `json:"vm_name"`
	HypervisorID      string    `json:"hypervisor_id,omitempty"`
	CookbookName      string    `json:"cookbook_name"`
	SuiteName         string    `json:"suite_name"`
	PlatformName      string    `json:"platform_name"`
	BatchID           string    `json:"batch_id,omitempty"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	ExpectedDestroyAt time.Time `json:"expected_destroy_at,omitempty"`
	ActualDestroyAt   time.Time `json:"actual_destroy_at,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// InsertTrackedVMParams holds the fields needed to create a new VM tracking
// record.
type InsertTrackedVMParams struct {
	VMName            string
	HypervisorID      string
	CookbookName      string
	SuiteName         string
	PlatformName      string
	BatchID           string
	ExpectedDestroyAt time.Time
}

// ---------------------------------------------------------------------------
// Status constants and validation
// ---------------------------------------------------------------------------

var validVMStatuses = map[string]bool{
	"creating":   true,
	"running":    true,
	"destroying": true,
	"destroyed":  true,
	"orphaned":   true,
}

func validateVMStatus(status string) error {
	if !validVMStatuses[status] {
		return fmt.Errorf("datastore: invalid VM status %q", status)
	}
	return nil
}

func validateInsertTrackedVMParams(p InsertTrackedVMParams) error {
	if p.VMName == "" {
		return fmt.Errorf("datastore: vm_name is required")
	}
	if p.CookbookName == "" {
		return fmt.Errorf("datastore: cookbook_name is required")
	}
	if p.SuiteName == "" {
		return fmt.Errorf("datastore: suite_name is required")
	}
	if p.PlatformName == "" {
		return fmt.Errorf("datastore: platform_name is required")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Column list
// ---------------------------------------------------------------------------

const vmTrackingColumns = `id, vm_name, hypervisor_id, cookbook_name, suite_name, platform_name,
    batch_id, status, created_at, expected_destroy_at, actual_destroy_at, updated_at`

// ---------------------------------------------------------------------------
// Insert
// ---------------------------------------------------------------------------

// InsertTrackedVM creates a new VM tracking record with status 'creating' and
// returns the full row.
func (db *DB) InsertTrackedVM(ctx context.Context, p InsertTrackedVMParams) (*TrackedVM, error) {
	if err := validateInsertTrackedVMParams(p); err != nil {
		return nil, err
	}

	const query = `
		INSERT INTO vm_tracking (
			vm_name, hypervisor_id, cookbook_name, suite_name, platform_name,
			batch_id, expected_destroy_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + vmTrackingColumns

	r, err := scanTrackedVM(db.q().QueryRowContext(ctx, query,
		p.VMName,
		nullString(p.HypervisorID),
		p.CookbookName,
		p.SuiteName,
		p.PlatformName,
		nullString(p.BatchID),
		nullTime(p.ExpectedDestroyAt),
	))
	if err != nil {
		return nil, fmt.Errorf("datastore: inserting tracked VM: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetTrackedVM returns a single VM by its primary key UUID.
// Returns (nil, nil) if not found.
func (db *DB) GetTrackedVM(ctx context.Context, id string) (*TrackedVM, error) {
	if id == "" {
		return nil, fmt.Errorf("datastore: vm id is required")
	}

	const query = `SELECT ` + vmTrackingColumns + ` FROM vm_tracking WHERE id = $1`

	r, err := scanTrackedVM(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting tracked VM: %w", err)
	}
	return &r, nil
}

// GetTrackedVMByName returns the most recently created VM with the given name.
// Returns (nil, nil) if not found.
func (db *DB) GetTrackedVMByName(ctx context.Context, vmName string) (*TrackedVM, error) {
	if vmName == "" {
		return nil, fmt.Errorf("datastore: vm_name is required")
	}

	const query = `SELECT ` + vmTrackingColumns + `
		FROM vm_tracking WHERE vm_name = $1
		ORDER BY created_at DESC LIMIT 1`

	r, err := scanTrackedVM(db.q().QueryRowContext(ctx, query, vmName))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting tracked VM by name: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// UpdateTrackedVMStatus updates a VM's status and optionally its
// hypervisor_id. The updated_at timestamp is set to now().
func (db *DB) UpdateTrackedVMStatus(ctx context.Context, id string, status string, hypervisorID string) error {
	if id == "" {
		return fmt.Errorf("datastore: vm id is required")
	}
	if err := validateVMStatus(status); err != nil {
		return err
	}

	const query = `
		UPDATE vm_tracking
		   SET status = $1,
		       hypervisor_id = COALESCE(NULLIF($2, ''), hypervisor_id),
		       updated_at = now()
		 WHERE id = $3`

	res, err := db.q().ExecContext(ctx, query, status, hypervisorID, id)
	if err != nil {
		return fmt.Errorf("datastore: updating tracked VM status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkVMDestroyed sets a VM's status to 'destroyed' and records the actual
// destroy time as now().
func (db *DB) MarkVMDestroyed(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("datastore: vm id is required")
	}

	const query = `
		UPDATE vm_tracking
		   SET status = 'destroyed',
		       actual_destroy_at = now(),
		       updated_at = now()
		 WHERE id = $1`

	res, err := db.q().ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: marking VM destroyed: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkVMOrphaned sets a VM's status to 'orphaned'.
func (db *DB) MarkVMOrphaned(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("datastore: vm id is required")
	}

	const query = `
		UPDATE vm_tracking
		   SET status = 'orphaned',
		       updated_at = now()
		 WHERE id = $1`

	res, err := db.q().ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: marking VM orphaned: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListTrackedVMs returns all VMs ordered by created_at DESC.
func (db *DB) ListTrackedVMs(ctx context.Context) ([]TrackedVM, error) {
	const query = `SELECT ` + vmTrackingColumns + `
		FROM vm_tracking ORDER BY created_at DESC`

	return scanTrackedVMs(db.q().QueryContext(ctx, query))
}

// ListTrackedVMsFiltered returns VMs filtered by status. An empty status
// returns all VMs. Ordered by created_at DESC.
func (db *DB) ListTrackedVMsFiltered(ctx context.Context, status string) ([]TrackedVM, error) {
	if status == "" {
		return db.ListTrackedVMs(ctx)
	}

	const query = `SELECT ` + vmTrackingColumns + `
		FROM vm_tracking WHERE status = $1 ORDER BY created_at DESC`

	return scanTrackedVMs(db.q().QueryContext(ctx, query, status))
}

// ListOrphanedVMs returns VMs that have passed their expected destroy time
// but are not yet destroyed or marked orphaned. These are candidates for
// orphan flagging.
func (db *DB) ListOrphanedVMs(ctx context.Context) ([]TrackedVM, error) {
	const query = `SELECT ` + vmTrackingColumns + `
		FROM vm_tracking
		WHERE status NOT IN ('destroyed', 'orphaned')
		  AND expected_destroy_at IS NOT NULL
		  AND expected_destroy_at < now()
		ORDER BY created_at DESC`

	return scanTrackedVMs(db.q().QueryContext(ctx, query))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteTrackedVM removes a VM tracking record by id.
// Returns ErrNotFound if no row was deleted.
func (db *DB) DeleteTrackedVM(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("datastore: vm id is required")
	}

	const query = `DELETE FROM vm_tracking WHERE id = $1`

	res, err := db.q().ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting tracked VM: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Aggregation
// ---------------------------------------------------------------------------

// CountTrackedVMsByStatus returns a map of status → count for all VMs.
func (db *DB) CountTrackedVMsByStatus(ctx context.Context) (map[string]int, error) {
	const query = `SELECT status, COUNT(*) FROM vm_tracking GROUP BY status`

	rows, err := db.q().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("datastore: counting tracked VMs by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("datastore: scanning VM status count: %w", err)
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating VM status counts: %w", err)
	}
	return counts, nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanTrackedVM(row *sql.Row) (TrackedVM, error) {
	var vm TrackedVM
	var hypervisorID, batchID sql.NullString
	var expectedDestroyAt, actualDestroyAt sql.NullTime

	err := row.Scan(
		&vm.ID,
		&vm.VMName,
		&hypervisorID,
		&vm.CookbookName,
		&vm.SuiteName,
		&vm.PlatformName,
		&batchID,
		&vm.Status,
		&vm.CreatedAt,
		&expectedDestroyAt,
		&actualDestroyAt,
		&vm.UpdatedAt,
	)
	if err != nil {
		return TrackedVM{}, err
	}

	vm.HypervisorID = stringFromNull(hypervisorID)
	vm.BatchID = stringFromNull(batchID)
	vm.ExpectedDestroyAt = timeFromNull(expectedDestroyAt)
	vm.ActualDestroyAt = timeFromNull(actualDestroyAt)

	return vm, nil
}

func scanTrackedVMs(rows *sql.Rows, err error) ([]TrackedVM, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: listing tracked VMs: %w", err)
	}
	defer rows.Close()

	var results []TrackedVM
	for rows.Next() {
		var vm TrackedVM
		var hypervisorID, batchID sql.NullString
		var expectedDestroyAt, actualDestroyAt sql.NullTime

		if err := rows.Scan(
			&vm.ID,
			&vm.VMName,
			&hypervisorID,
			&vm.CookbookName,
			&vm.SuiteName,
			&vm.PlatformName,
			&batchID,
			&vm.Status,
			&vm.CreatedAt,
			&expectedDestroyAt,
			&actualDestroyAt,
			&vm.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning tracked VM row: %w", err)
		}

		vm.HypervisorID = stringFromNull(hypervisorID)
		vm.BatchID = stringFromNull(batchID)
		vm.ExpectedDestroyAt = timeFromNull(expectedDestroyAt)
		vm.ActualDestroyAt = timeFromNull(actualDestroyAt)

		results = append(results, vm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating tracked VM rows: %w", err)
	}
	return results, nil
}
