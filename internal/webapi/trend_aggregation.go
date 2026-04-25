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
	WarningNodes     int    `json:"warning_nodes"`
	CriticalNodes    int    `json:"critical_nodes"`
}

// complexityTrendPoint is a single data point in the complexity trend
// response. Each point represents aggregate complexity scores for one
// (organisation, target version) pair from one collection run.
type complexityTrendPoint struct {
	OrganisationName  string  `json:"organisation_name"`
	CollectionRunOrg  string  `json:"collection_run_org"`
	CompletedAt       string  `json:"completed_at"`
	TargetChefVersion string  `json:"target_chef_version"`
	TotalCookbooks    int     `json:"total_cookbooks"`
	TotalScore        int     `json:"total_score"`
	AverageScore      float64 `json:"average_score"`
	LowCount          int     `json:"low_count"`
	MediumCount       int     `json:"medium_count"`
	HighCount         int     `json:"high_count"`
	CriticalCount     int     `json:"critical_count"`
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

// orgHourKey is used to deduplicate snapshots per org within an hour bucket.
type orgHourKey struct {
	hour time.Time
	org  string
}

// mergeVersionDistributionSnapshots aggregates per-org version distribution
// trend points into one merged point per hour bucket. Within each hour,
// only the latest snapshot per org is kept (to avoid double-counting when
// an org has multiple snapshots in the same hour). Then distributions and
// TotalNodes are summed across orgs. The returned slice is sorted ascending
// by time.
func mergeVersionDistributionSnapshots(points []versionDistTrendPoint) []versionDistTrendPoint {
	if len(points) == 0 {
		return points
	}

	// Phase 1: Deduplicate — keep the latest snapshot per (org, hour).
	type dedupEntry struct {
		point     versionDistTrendPoint
		timestamp time.Time
	}
	deduped := make(map[orgHourKey]*dedupEntry)

	for _, p := range points {
		t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
		if err != nil {
			continue
		}
		hour := truncateToHour(t)
		key := orgHourKey{hour: hour, org: p.OrganisationName}

		if existing, ok := deduped[key]; !ok || t.After(existing.timestamp) {
			deduped[key] = &dedupEntry{point: p, timestamp: t}
		}
	}

	// Phase 2: Sum across orgs within each hour bucket.
	type bucket struct {
		totalNodes   int
		distribution map[string]int
	}

	buckets := make(map[time.Time]*bucket)
	var hours []time.Time

	for k, entry := range deduped {
		b, ok := buckets[k.hour]
		if !ok {
			b = &bucket{distribution: make(map[string]int)}
			buckets[k.hour] = b
			hours = append(hours, k.hour)
		}
		b.totalNodes += entry.point.TotalNodes
		for ver, cnt := range entry.point.Distribution {
			b.distribution[ver] += cnt
		}
	}

	sort.Slice(hours, func(i, j int) bool { return hours[i].Before(hours[j]) })

	result := make([]versionDistTrendPoint, 0, len(hours))
	for _, hour := range hours {
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
// merged point per hour bucket. Within each hour, only the latest snapshot
// per org is kept. Then TotalNodes, StaleNodes, and FreshNodes are summed
// across orgs. The returned slice is sorted ascending by time.
func mergeStaleTrendSnapshots(points []staleTrendPoint) []staleTrendPoint {
	if len(points) == 0 {
		return points
	}

	// Phase 1: Deduplicate — keep the latest snapshot per (org, hour).
	type dedupEntry struct {
		point     staleTrendPoint
		timestamp time.Time
	}
	deduped := make(map[orgHourKey]*dedupEntry)

	for _, p := range points {
		t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
		if err != nil {
			continue
		}
		hour := truncateToHour(t)
		key := orgHourKey{hour: hour, org: p.OrganisationName}

		if existing, ok := deduped[key]; !ok || t.After(existing.timestamp) {
			deduped[key] = &dedupEntry{point: p, timestamp: t}
		}
	}

	// Phase 2: Sum across orgs within each hour bucket.
	type bucket struct {
		totalNodes    int
		staleNodes    int
		freshNodes    int
		warningNodes  int
		criticalNodes int
	}

	buckets := make(map[time.Time]*bucket)
	var hours []time.Time

	for k, entry := range deduped {
		b, ok := buckets[k.hour]
		if !ok {
			b = &bucket{}
			buckets[k.hour] = b
			hours = append(hours, k.hour)
		}
		b.totalNodes += entry.point.TotalNodes
		b.staleNodes += entry.point.StaleNodes
		b.freshNodes += entry.point.FreshNodes
		b.warningNodes += entry.point.WarningNodes
		b.criticalNodes += entry.point.CriticalNodes
	}

	sort.Slice(hours, func(i, j int) bool { return hours[i].Before(hours[j]) })

	result := make([]staleTrendPoint, 0, len(hours))
	for _, hour := range hours {
		b := buckets[hour]
		result = append(result, staleTrendPoint{
			CompletedAt:   hour.Format(trendTimestampFormat),
			TotalNodes:    b.totalNodes,
			StaleNodes:    b.staleNodes,
			FreshNodes:    b.freshNodes,
			WarningNodes:  b.warningNodes,
			CriticalNodes: b.criticalNodes,
		})
	}
	return result
}

// complexityOrgKey deduplicates complexity snapshots per (org, hour, version).
type complexityOrgKey struct {
	hour    time.Time
	org     string
	version string
}

// complexityKey groups complexity trend points by hour and target version.
type complexityKey struct {
	hour    time.Time
	version string
}

// readinessKey groups readiness trend points by hour and target version.
type readinessKey struct {
	hour    time.Time
	version string
}

// readinessOrgKey deduplicates readiness snapshots per (org, hour, version).
type readinessOrgKey struct {
	hour    time.Time
	org     string
	version string
}

// mergeComplexityTrendSnapshots aggregates per-org complexity trend points
// into one merged point per (hour, target version) pair. Within each
// bucket, only the latest snapshot per org is kept. Then TotalCookbooks,
// TotalScore, LowCount, MediumCount, HighCount, and CriticalCount are
// summed, and AverageScore is recomputed. The returned slice is sorted
// ascending by (time, version).
func mergeComplexityTrendSnapshots(points []complexityTrendPoint) []complexityTrendPoint {
	if len(points) == 0 {
		return points
	}

	// Phase 1: Deduplicate — keep the latest snapshot per (org, hour, version).
	type dedupEntry struct {
		point     complexityTrendPoint
		timestamp time.Time
	}
	deduped := make(map[complexityOrgKey]*dedupEntry)

	for _, p := range points {
		t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
		if err != nil {
			continue
		}
		hour := truncateToHour(t)
		key := complexityOrgKey{hour: hour, org: p.OrganisationName, version: p.TargetChefVersion}

		if existing, ok := deduped[key]; !ok || t.After(existing.timestamp) {
			deduped[key] = &dedupEntry{point: p, timestamp: t}
		}
	}

	// Phase 2: Sum across orgs within each (hour, version) bucket.
	type bucket struct {
		totalCookbooks int
		totalScore     int
		lowCount       int
		mediumCount    int
		highCount      int
		criticalCount  int
	}

	buckets := make(map[complexityKey]*bucket)
	var keys []complexityKey

	for k, entry := range deduped {
		ck := complexityKey{hour: k.hour, version: k.version}
		b, ok := buckets[ck]
		if !ok {
			b = &bucket{}
			buckets[ck] = b
			keys = append(keys, ck)
		}
		b.totalCookbooks += entry.point.TotalCookbooks
		b.totalScore += entry.point.TotalScore
		b.lowCount += entry.point.LowCount
		b.mediumCount += entry.point.MediumCount
		b.highCount += entry.point.HighCount
		b.criticalCount += entry.point.CriticalCount
	}

	sort.Slice(keys, func(i, j int) bool {
		if !keys[i].hour.Equal(keys[j].hour) {
			return keys[i].hour.Before(keys[j].hour)
		}
		return keys[i].version < keys[j].version
	})

	result := make([]complexityTrendPoint, 0, len(keys))
	for _, k := range keys {
		b := buckets[k]
		avg := 0.0
		if b.totalCookbooks > 0 {
			avg = float64(b.totalScore) / float64(b.totalCookbooks)
		}
		result = append(result, complexityTrendPoint{
			CompletedAt:       k.hour.Format(trendTimestampFormat),
			TargetChefVersion: k.version,
			TotalCookbooks:    b.totalCookbooks,
			TotalScore:        b.totalScore,
			AverageScore:      avg,
			LowCount:          b.lowCount,
			MediumCount:       b.mediumCount,
			HighCount:         b.highCount,
			CriticalCount:     b.criticalCount,
		})
	}
	return result
}

// mergeReadinessTrendSnapshots aggregates per-org readiness trend points
// into one merged point per (hour, target version) pair. Within each
// bucket, only the latest snapshot per org is kept. Then TotalNodes,
// ReadyNodes, and BlockedNodes are summed, and ReadyPercent is recomputed.
// The returned slice is sorted ascending by (time, version).
func mergeReadinessTrendSnapshots(points []readinessTrendPoint) []readinessTrendPoint {
	if len(points) == 0 {
		return points
	}

	// Phase 1: Deduplicate — keep the latest snapshot per (org, hour, version).
	type dedupEntry struct {
		point     readinessTrendPoint
		timestamp time.Time
	}
	deduped := make(map[readinessOrgKey]*dedupEntry)

	for _, p := range points {
		t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
		if err != nil {
			continue
		}
		hour := truncateToHour(t)
		key := readinessOrgKey{hour: hour, org: p.OrganisationName, version: p.TargetChefVersion}

		if existing, ok := deduped[key]; !ok || t.After(existing.timestamp) {
			deduped[key] = &dedupEntry{point: p, timestamp: t}
		}
	}

	// Phase 2: Sum across orgs within each (hour, version) bucket.
	type bucket struct {
		totalNodes   int
		readyNodes   int
		blockedNodes int
	}

	buckets := make(map[readinessKey]*bucket)
	var keys []readinessKey

	for k, entry := range deduped {
		rk := readinessKey{hour: k.hour, version: k.version}
		b, ok := buckets[rk]
		if !ok {
			b = &bucket{}
			buckets[rk] = b
			keys = append(keys, rk)
		}
		b.totalNodes += entry.point.TotalNodes
		b.readyNodes += entry.point.ReadyNodes
		b.blockedNodes += entry.point.BlockedNodes
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
