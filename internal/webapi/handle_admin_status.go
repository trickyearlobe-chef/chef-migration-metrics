// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/migrations"
)

// ---------------------------------------------------------------------------
// GET /api/v1/admin/status
//
// Operational health snapshot for support bundles / screenshots: datastore
// connectivity + pending migrations, credential-storage state, collection
// scheduling, and per-organisation collection status. Always HTTP 200 — health
// is reported in the body's `status` field, never via the HTTP status code, so
// a monitoring client can always parse the payload. Contract:
// specifications/web-api-admin.md §`GET /api/v1/admin/status`.
// ---------------------------------------------------------------------------

type adminStatusResponse struct {
	Status            string                     `json:"status"`
	Version           string                     `json:"version"`
	Datastore         adminStatusDatastore       `json:"datastore"`
	CredentialStorage adminStatusCredentialStore `json:"credential_storage"`
	Collection        adminStatusCollection      `json:"collection"`
	Organisations     []adminStatusOrganisation  `json:"organisations"`
}

type adminStatusDatastore struct {
	Status            string `json:"status"`
	PendingMigrations int    `json:"pending_migrations"`
}

type adminStatusCredentialStore struct {
	EncryptionKeyConfigured bool           `json:"encryption_key_configured"`
	TotalCredentials        int            `json:"total_credentials"`
	CredentialTypes         map[string]int `json:"credential_types"`
	OrphanedCredentials     int            `json:"orphaned_credentials"`
}

type adminStatusCollection struct {
	NextRunAt     *time.Time `json:"next_run_at"`
	LastRunAt     *time.Time `json:"last_run_at"`
	LastRunStatus string     `json:"last_run_status"`
}

type adminStatusOrganisation struct {
	Name             string     `json:"name"`
	CredentialSource string     `json:"credential_source"`
	LastCollectedAt  *time.Time `json:"last_collected_at"`
	Status           string     `json:"status"`
	NodeCount        int        `json:"node_count"`
}

func (r *Router) handleAdminStatus(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET.")
		return
	}

	ctx := req.Context()
	resp := adminStatusResponse{
		Version:           r.version,
		CredentialStorage: adminStatusCredentialStore{CredentialTypes: map[string]int{}},
		Organisations:     []adminStatusOrganisation{},
	}

	// --- datastore: connectivity + pending migrations ---
	dbConnected := r.db.Ping(ctx) == nil
	resp.Datastore.Status = "connected"
	if !dbConnected {
		resp.Datastore.Status = "error"
	}
	resp.Datastore.PendingMigrations = r.pendingMigrations(ctx)

	// --- credential storage ---
	r.fillCredentialStorage(ctx, &resp.CredentialStorage)

	// --- collection scheduling + per-org status ---
	r.fillCollectionStatus(ctx, &resp)

	// Overall health: a missing encryption key is a valid (file-credential)
	// deployment, so it does NOT count as degraded — only an unreachable
	// datastore or unapplied migrations do.
	resp.Status = "healthy"
	if !dbConnected || resp.Datastore.PendingMigrations > 0 {
		resp.Status = "degraded"
	}

	WriteJSON(w, http.StatusOK, resp)
}

// pendingMigrations reports how many embedded up-migrations have not yet been
// applied to the database, floored at zero (a database ahead of the binary —
// e.g. during a rollback — is reported as zero, not negative).
func (r *Router) pendingMigrations(ctx context.Context) int {
	applied, err := r.db.ListAppliedMigrations(ctx)
	if err != nil {
		r.logf("ERROR", "admin/status: listing applied migrations: %v", err)
		return 0
	}
	pending := countExpectedMigrations() - len(applied)
	if pending < 0 {
		return 0
	}
	return pending
}

// countExpectedMigrations counts the embedded `*.up.sql` migration files — the
// full set the binary expects to be applied.
func countExpectedMigrations() int {
	entries, err := fs.ReadDir(migrations.FS(), ".")
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			n++
		}
	}
	return n
}

// fillCredentialStorage populates the credential-storage section. When no
// credential store is wired (CMM_CREDENTIAL_ENCRYPTION_KEY unset) the section
// reports encryption_key_configured=false with zero counts.
func (r *Router) fillCredentialStorage(ctx context.Context, out *adminStatusCredentialStore) {
	if r.credentialStore == nil {
		return
	}
	out.EncryptionKeyConfigured = true

	creds, err := r.credentialStore.List(ctx)
	if err != nil {
		r.logf("ERROR", "admin/status: listing credentials: %v", err)
		return
	}
	out.TotalCredentials = len(creds)
	for i := range creds {
		out.CredentialTypes[creds[i].CredentialType]++
		// A credential nothing references is a cleanup candidate. The table is
		// small (matching handleListCredentials' in-memory pagination), so the
		// per-credential reference check is acceptable.
		refs, refErr := r.credentialStore.ReferencedBy(ctx, creds[i].Name)
		if refErr != nil {
			r.logf("ERROR", "admin/status: checking references for %q: %v", creds[i].Name, refErr)
			continue
		}
		if len(refs) == 0 {
			out.OrphanedCredentials++
		}
	}
}

// fillCollectionStatus populates next_run_at (from the live cron schedule),
// the per-organisation collection rows, and the most-recent run across all
// organisations.
func (r *Router) fillCollectionStatus(ctx context.Context, resp *adminStatusResponse) {
	if sched, err := collector.ParseSchedule(r.liveConfig().Collection.Schedule); err == nil {
		next := sched.Next(time.Now())
		resp.Collection.NextRunAt = &next
	}

	orgs, err := r.db.ListOrganisations(ctx)
	if err != nil {
		r.logf("ERROR", "admin/status: listing organisations: %v", err)
		return
	}

	var lastRunAt time.Time
	var lastRunStatus string
	for _, o := range orgs {
		row := adminStatusOrganisation{Name: o.Name, CredentialSource: "file"}
		if o.ClientKeyCredentialName != "" {
			row.CredentialSource = "database"
		}

		run, runErr := r.db.GetLatestCollectionRun(ctx, o.Name)
		switch {
		case runErr == nil:
			row.Status = run.Status
			row.NodeCount = run.TotalNodes
			if !run.CompletedAt.IsZero() {
				completed := run.CompletedAt
				row.LastCollectedAt = &completed
			}
			if run.StartedAt.After(lastRunAt) {
				lastRunAt = run.StartedAt
				lastRunStatus = run.Status
			}
		case errors.Is(runErr, datastore.ErrNotFound):
			row.Status = "never_collected"
		default:
			r.logf("ERROR", "admin/status: latest run for %q: %v", o.Name, runErr)
			row.Status = "unknown"
		}

		resp.Organisations = append(resp.Organisations, row)
	}

	if !lastRunAt.IsZero() {
		resp.Collection.LastRunAt = &lastRunAt
		resp.Collection.LastRunStatus = lastRunStatus
	}
}
