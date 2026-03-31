// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"sort"
	"time"
)

// ---------------------------------------------------------------------------
// Shared trend point types (extracted from handler anonymous structs)
// ---------------------------------------------------------------------------

// versionDistTrendPoint is a single data point in the version distribution
// trend response. Each point represents one collection run's version
// distribution for an organisation.
type versionDistTrendPoint struct {
	OrganisationName string         `json:"organisation_name"`
	CollectionRunOrg string         `json:"collection_run_org"`
	CompletedAt      string         `json:"completed_at"`
	TotalNodes       int            `json:"total_nodes"`
	Distribution     map[string]int `json:"distribution"`
}

// staleTrendPoint is a single data point in the stale node trend response.
// Each point represents stale vs. fresh counts from one collection run.
type staleTrendPoint struct {
	OrganisationName string `json:"organisation_name"`
	CollectionRunOrg string `json:"collection_run_org"`
	CompletedAt      string `json:"completed_at"`
	TotalNodes       int    `json:"total_nodes"`
	StaleNodes       int    `json:"stale_nodes"`
	FreshNodes       int    `json:"fresh_nodes"`
}

// readinessTrendPoint is a single data point in the readiness trend
// response. Each point represents readiness counts for one (organisation,
// target version) pair from one collection run.
type readinessTrendPoint struct {
	OrganisationName  string  `json:"organisation_name"`
	CollectionRunOrg  string  `json:"collection_run_org"`
	CompletedAt       string  `json:"completed_at"`
	TargetChefVersion string  `json:"target_chef_version"`
	TotalNodes        int     `json:"total_nodes"`
	ReadyNodes        int     `json:"ready_nodes"`
	BlockedNodes      int     `json:"blocked_nodes"`
	ReadyPercent      float64 `json:"ready_percent"`
}

// trendTimestampFormat is the time layout used in trend point timestamps.
const trendTimestampFormat = "2006-01-02T15:04:05Z"

// truncateToHour returns t truncated to the start of its hour in UTC.
func truncateToHour(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
}

// mergeVersionDistributionSnapshots aggregates per-org version distribution
// trend points into one merged point per hour bucket. Distribution map
// values and TotalNodes are summed across orgs within each bucket. The
// returned slice is sorted ascending by time.
func mergeVersionDistributionSnapshots(points []versionDistTrendPoint) []versionDistTrendPoint {
	if len(points) == 0 {
		return points
	}

	type bucket struct {
		totalNodes   int
		distribution map[string]int
	}

	buckets := make(map[time.Time]*bucket)
	var keys []time.Time

	for _, p := range points {
		t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
		if err != nil {
			continue
		}
		hour := truncateToHour(t)

		b, ok := buckets[hour]
		if !ok {
			b = &bucket{distribution: make(map[string]int)}
			buckets[hour] = b
			keys = append(keys, hour)
		}
		b.totalNodes += p.TotalNodes
		for ver, cnt := range p.Distribution {
			b.distribution[ver] += cnt
		}
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })

	result := make([]versionDistTrendPoint, 0, len(keys))
	for _, hour := range keys {
		b := buckets[hour]
		result = append(result, versionDistTrendPoint{
			CompletedAt:  hour.Format(trendTimestampFormat),
			TotalNodes:   b.totalNodes,
			Distribution: b.distribution,
		})
	}
	return result
}

// mergeStaleTrendSnapshots aggregates per-org stale trend points into one
// merged point per hour bucket. TotalNodes, StaleNodes, and FreshNodes are
// summed across orgs within each bucket. The returned slice is sorted
// ascending by time.
func mergeStaleTrendSnapshots(points []staleTrendPoint) []staleTrendPoint {
	if len(points) == 0 {
		return points
	}

	type bucket struct {
		totalNodes int
		staleNodes int
		freshNodes int
	}

	buckets := make(map[time.Time]*bucket)
	var keys []time.Time

	for _, p := range points {
		t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
		if err != nil {
			continue
		}
		hour := truncateToHour(t)

		b, ok := buckets[hour]
		if !ok {
			b = &bucket{}
			buckets[hour] = b
			keys = append(keys, hour)
		}
		b.totalNodes += p.TotalNodes
		b.staleNodes += p.StaleNodes
		b.freshNodes += p.FreshNodes
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })

	result := make([]staleTrendPoint, 0, len(keys))
	for _, hour := range keys {
		b := buckets[hour]
		result = append(result, staleTrendPoint{
			CompletedAt: hour.Format(trendTimestampFormat),
			TotalNodes:  b.totalNodes,
			StaleNodes:  b.staleNodes,
			FreshNodes:  b.freshNodes,
		})
	}
	return result
}

// readinessKey groups readiness trend points by hour and target version.
type readinessKey struct {
	hour    time.Time
	version string
}

// mergeReadinessTrendSnapshots aggregates per-org readiness trend points
// into one merged point per (hour, target version) pair. TotalNodes,
// ReadyNodes, and BlockedNodes are summed, and ReadyPercent is recomputed.
// The returned slice is sorted ascending by (time, version).
func mergeReadinessTrendSnapshots(points []readinessTrendPoint) []readinessTrendPoint {
	if len(points) == 0 {
		return points
	}

	type bucket struct {
		totalNodes   int
		readyNodes   int
		blockedNodes int
	}

	buckets := make(map[readinessKey]*bucket)
	var keys []readinessKey

	for _, p := range points {
		t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
		if err != nil {
			continue
		}
		k := readinessKey{
			hour:    truncateToHour(t),
			version: p.TargetChefVersion,
		}

		b, ok := buckets[k]
		if !ok {
			b = &bucket{}
			buckets[k] = b
			keys = append(keys, k)
		}
		b.totalNodes += p.TotalNodes
		b.readyNodes += p.ReadyNodes
		b.blockedNodes += p.BlockedNodes
	}

	sort.Slice(keys, func(i, j int) bool {
		if !keys[i].hour.Equal(keys[j].hour) {
			return keys[i].hour.Before(keys[j].hour)
		}
		return keys[i].version < keys[j].version
	})

	result := make([]readinessTrendPoint, 0, len(keys))
	for _, k := range keys {
		b := buckets[k]
		pct := 0.0
		if b.totalNodes > 0 {
			pct = float64(b.readyNodes) / float64(b.totalNodes) * 100
		}
		result = append(result, readinessTrendPoint{
			CompletedAt:       k.hour.Format(trendTimestampFormat),
			TargetChefVersion: k.version,
			TotalNodes:        b.totalNodes,
			ReadyNodes:        b.readyNodes,
			BlockedNodes:      b.blockedNodes,
			ReadyPercent:      pct,
		})
	}
	return result
}
