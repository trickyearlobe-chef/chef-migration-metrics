// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/staleness"
)

// ---------------------------------------------------------------------------
// Node Metrics Snapshot — pure aggregate payload builder
// ---------------------------------------------------------------------------

// nodeMetricsPayload is the JSONB payload shape for the "node_metrics"
// snapshot type. It contains only pre-aggregated counts — no per-node data.
type nodeMetricsPayload struct {
	TotalNodes       int                `json:"total_nodes"`
	TargetChefVer    string             `json:"target_chef_version"`
	ByStaleness      stalenessBreakdown `json:"by_staleness"`
	Fresh            freshBreakdown     `json:"fresh"`
	Deployment       deploymentBreakdown `json:"deployment"`
	Thresholds       thresholdsRecord   `json:"thresholds"`
}

// stalenessBreakdown holds counts per staleness tier.
type stalenessBreakdown struct {
	Fresh    int `json:"fresh"`
	Warning  int `json:"warning"`
	Critical int `json:"critical"`
}

// freshBreakdown holds readiness and distribution data for fresh nodes only.
type freshBreakdown struct {
	Total            int            `json:"total"`
	Ready            int            `json:"ready"`
	BlockedTotal     int            `json:"blocked_total"`
	BlockedBy        blockedByCount `json:"blocked_by"`
	ByVersion        map[string]int `json:"by_version"`
	ByPlatformFamily map[string]int `json:"by_platform_family"`
}

// blockedByCount tracks how many fresh nodes are blocked by each check type.
// A node can be counted in multiple categories.
type blockedByCount struct {
	Cookstyle   int `json:"cookstyle"`
	TestKitchen int `json:"test_kitchen"`
	Disk        int `json:"disk"`
	FoodCritic  int `json:"foodcritic"`
	ChefSpec    int `json:"chefspec"`
}

// deploymentBreakdown holds parallel deployment progress counts across all nodes.
type deploymentBreakdown struct {
	StagedOrActivated int                                `json:"staged_or_activated"`
	ConvergePassing   int                                `json:"converge_passing"`
	ByVersion         map[string]deploymentVersionBreakdown `json:"by_version"`
}

// deploymentVersionBreakdown holds per-version deployment state counts.
type deploymentVersionBreakdown struct {
	Staged          int `json:"staged"`
	Activated       int `json:"activated"`
	ConvergePassing int `json:"converge_passing"`
	ConvergeFailing int `json:"converge_failing"`
}

// thresholdsRecord captures the configuration at collection time.
type thresholdsRecord struct {
	WarningHours   int `json:"warning_hours"`
	CriticalDays   int `json:"critical_days"`
	RequiredDiskMB int `json:"required_disk_mb"`
}

// nodeMetricsInput bundles all data needed to build the node_metrics payload.
type nodeMetricsInput struct {
	SnapshotParams    []datastore.InsertNodeSnapshotParams
	ReadinessResults  []analysis.ReadinessResult
	TargetChefVersion string
	WarningHours      int
	CriticalDays      int
	RequiredDiskMB    int
	Now               time.Time
}

// buildNodeMetricsPayload builds the pure-aggregate JSONB payload for a
// "node_metrics" metric snapshot. It computes staleness tiers, readiness,
// blocking reasons, and version/platform distributions for fresh nodes.
func buildNodeMetricsPayload(input nodeMetricsInput) (json.RawMessage, error) {
	thresholds := staleness.Thresholds{
		WarningHours: input.WarningHours,
		CriticalDays: input.CriticalDays,
	}

	// Build readiness lookup by node name for O(1) access.
	readinessMap := make(map[string]*analysis.ReadinessResult, len(input.ReadinessResults))
	for i := range input.ReadinessResults {
		readinessMap[input.ReadinessResults[i].NodeName] = &input.ReadinessResults[i]
	}

	payload := nodeMetricsPayload{
		TotalNodes:    len(input.SnapshotParams),
		TargetChefVer: input.TargetChefVersion,
		Fresh: freshBreakdown{
			ByVersion:        make(map[string]int),
			ByPlatformFamily: make(map[string]int),
		},
		Deployment: deploymentBreakdown{
			ByVersion: make(map[string]deploymentVersionBreakdown),
		},
		Thresholds: thresholdsRecord{
			WarningHours:   input.WarningHours,
			CriticalDays:   input.CriticalDays,
			RequiredDiskMB: input.RequiredDiskMB,
		},
	}

	for _, p := range input.SnapshotParams {
		// Deployment progress — applies to all nodes regardless of staleness.
		if p.MigrationState == "hab_dormant" || p.MigrationState == "hab_active" {
			payload.Deployment.StagedOrActivated++

			// Determine the deployed version for per-version breakdown.
			var deployedVersion string
			if p.MigrationState == "hab_dormant" {
				deployedVersion = p.DormantChefVersion
			} else {
				deployedVersion = p.ActiveChefVersion
			}

			if deployedVersion != "" {
				vb := payload.Deployment.ByVersion[deployedVersion]
				if p.MigrationState == "hab_dormant" {
					vb.Staged++
				} else {
					vb.Activated++
				}
				if p.TargetConvergeStatus == "success" {
					vb.ConvergePassing++
				} else if p.TargetConvergeStatus == "failed" {
					vb.ConvergeFailing++
				}
				payload.Deployment.ByVersion[deployedVersion] = vb
			}
		}
		if p.TargetConvergeStatus == "success" {
			payload.Deployment.ConvergePassing++
		}

		// Compute staleness tier for this node.
		var ohaiTime time.Time
		if p.OhaiTime > 0 {
			ohaiTime = time.Unix(int64(p.OhaiTime), 0)
		}
		tier := staleness.ComputeTier(ohaiTime, input.Now, thresholds)

		switch tier {
		case staleness.Fresh:
			payload.ByStaleness.Fresh++
		case staleness.Warning:
			payload.ByStaleness.Warning++
		case staleness.Critical:
			payload.ByStaleness.Critical++
		}

		// Only compute readiness/version/platform for fresh nodes.
		if tier != staleness.Fresh {
			continue
		}

		payload.Fresh.Total++

		// Version distribution.
		ver := p.ChefVersion
		if ver == "" {
			ver = "unknown"
		}
		payload.Fresh.ByVersion[ver]++

		// Platform family distribution.
		pf := p.PlatformFamily
		if pf == "" {
			pf = "unknown"
		}
		payload.Fresh.ByPlatformFamily[pf]++

		// Readiness and blocking reasons.
		rr := readinessMap[p.NodeName]
		if rr == nil {
			// No readiness result — treat as blocked (unknown).
			payload.Fresh.BlockedTotal++
			continue
		}

		if rr.IsReady {
			payload.Fresh.Ready++
			continue
		}

		payload.Fresh.BlockedTotal++

		// Determine blocking reasons.
		blockedByCS, blockedByTK := classifyBlockingCookbooks(rr.BlockingCookbooks)
		if blockedByCS {
			payload.Fresh.BlockedBy.Cookstyle++
		}
		if blockedByTK {
			payload.Fresh.BlockedBy.TestKitchen++
		}

		// Disk blocking: not ready AND not all-cookbooks-compatible doesn't
		// necessarily mean disk is a problem. Disk blocks when space is
		// insufficient or unknown.
		diskOK := rr.SufficientDiskSpace != nil && *rr.SufficientDiskSpace
		if !diskOK {
			payload.Fresh.BlockedBy.Disk++
		}
	}

	return json.Marshal(payload)
}

// classifyBlockingCookbooks examines blocking cookbooks to determine whether
// the node is blocked by cookstyle, test kitchen, or both.
func classifyBlockingCookbooks(blocking []analysis.BlockingCookbook) (cookstyle, testKitchen bool) {
	for _, bc := range blocking {
		switch bc.Source {
		case analysis.SourceGitTestKitchen:
			testKitchen = true
		case analysis.SourceCookstyle, analysis.SourceServerCookstyle, analysis.SourceGitCookstyle:
			cookstyle = true
		case analysis.SourceNone:
			// Untested cookbook — counts as cookstyle (it's the static analysis that's missing).
			cookstyle = true
		}
	}
	return
}

// recordNodeMetricsSnapshot builds and persists the unified "node_metrics"
// snapshot for the organisation. Called after readiness evaluation completes.
func (c *Collector) recordNodeMetricsSnapshot(
	ctx context.Context,
	log *logging.ScopedLogger,
	collectionRunID string,
	organisationID string,
	snapshotParams []datastore.InsertNodeSnapshotParams,
	readinessResults []analysis.ReadinessResult,
) {
	targetVersion := ""
	if len(c.cfg.TargetChefVersions) > 0 {
		targetVersion = c.cfg.TargetChefVersions[0]
	}

	raw, err := buildNodeMetricsPayload(nodeMetricsInput{
		SnapshotParams:    snapshotParams,
		ReadinessResults:  readinessResults,
		TargetChefVersion: targetVersion,
		WarningHours:      c.cfg.Collection.StaleNodeWarningHours,
		CriticalDays:      c.cfg.Collection.StaleNodeCriticalDays,
		RequiredDiskMB:    c.cfg.Readiness.InstallSizeMBLinux,
		Now:               time.Now().UTC(),
	})
	if err != nil {
		log.Warn(fmt.Sprintf("failed to build node_metrics payload: %v", err),
			logging.WithCollectionRunID(collectionRunID))
		return
	}

	if _, msErr := c.db.InsertMetricSnapshot(ctx, datastore.InsertMetricSnapshotParams{
		CollectionRunOrg: collectionRunID,
		OrganisationName: organisationID,
		SnapshotType:     "node_metrics",
		Data:             raw,
		SnapshotAt:       time.Now().UTC(),
	}); msErr != nil {
		log.Warn(fmt.Sprintf("failed to record node_metrics snapshot: %v", msErr),
			logging.WithCollectionRunID(collectionRunID))
	}
}
