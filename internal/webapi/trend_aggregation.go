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
	OrganisationName  string             `json:"organisation_name"`
	CollectionRunOrg  string             `json:"collection_run_org"`
	CompletedAt       string             `json:"completed_at"`
	TargetChefVersion string             `json:"target_chef_version"`
	TotalNodes        int                `json:"total_nodes"`
	ReadyNodes        int                `json:"ready_nodes"`
	NeedsReviewNodes  int                `json:"needs_review_nodes,omitempty"`
	BlockedNodes      int                `json:"blocked_nodes"`
	ReadyPercent      float64            `json:"ready_percent"`
	BlockedBy         *blockedByResponse `json:"blocked_by,omitempty"`
	FilterLimited     bool               `json:"filter_limited,omitempty"`
}

// blockedByResponse provides a breakdown of why fresh nodes are blocked.
type blockedByResponse struct {
	Cookstyle   int `json:"cookstyle"`
	TestKitchen int `json:"test_kitchen"`
	Disk        int `json:"disk"`
	FoodCritic  int `json:"foodcritic"`
	ChefSpec    int `json:"chefspec"`
}

// trendTimestampFormat is the time layout used in trend point timestamps.
const trendTimestampFormat = "2006-01-02T15:04:05Z"

// truncateToHour returns t truncated to the start of its hour in UTC.
func truncateToHour(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
}

// fillBucketKey identifies an output bucket after forward-fill: an hour on the
// global time axis, plus an optional variant (the target Chef version for the
// version-scoped charts; "" for the fleet-wide ones).
type fillBucketKey struct {
	hour    time.Time
	variant string
}

// forwardFillContributions is the shared cross-org trend aggregation core.
//
// Orgs collect at slightly different times, so bucketing per hour and summing
// only the orgs that happen to have a snapshot in each bucket produces a
// seesaw: a bucket where only one org collected shows just that org's counts,
// dipping sharply before the next org's bucket. Selecting a single org hides
// this because there is no cross-org bucket mismatch.
//
// Instead, treat each (org, variant) as an independent step-function series and
// forward-fill it across the global hour axis: for every hour at or after a
// series' first sample, the series contributes its most recent sample at or
// before that hour. Summing these carried-forward values means every bucket
// reflects ALL orgs that have ever reported (their last-known state), not just
// those that collected in that hour — so the line is smooth whether orgs
// collect at different hours or skip a cycle entirely.
//
// A series is never back-filled before its first appearance (an org that didn't
// exist yet contributes nothing), and its last value is carried forward
// indefinitely (a decommissioned org keeps its final counts — an accepted
// trade-off; org removal is rare and out of scope for the trend view).
//
// Points are first deduplicated to the latest per (org, hour, variant). The
// returned map gives, per output bucket, the forward-filled contributing points
// (one per org); callers sum the fields they care about.
func forwardFillContributions[P any](
	points []P,
	tsOf func(P) (time.Time, bool),
	orgOf func(P) string,
	variantOf func(P) string,
) map[fillBucketKey][]P {
	type sample struct {
		hour  time.Time
		ts    time.Time
		point P
	}
	type dedupKey struct {
		hour    time.Time
		org     string
		variant string
	}
	type seriesKey struct {
		org     string
		variant string
	}

	// Phase 1: dedup to the latest sample per (org, hour, variant); collect the
	// global set of hours.
	latest := make(map[dedupKey]sample)
	hourSet := make(map[time.Time]struct{})
	for _, p := range points {
		t, ok := tsOf(p)
		if !ok {
			continue
		}
		t = t.UTC()
		hour := truncateToHour(t)
		hourSet[hour] = struct{}{}
		k := dedupKey{hour: hour, org: orgOf(p), variant: variantOf(p)}
		if ex, ok := latest[k]; !ok || t.After(ex.ts) {
			latest[k] = sample{hour: hour, ts: t, point: p}
		}
	}
	if len(latest) == 0 {
		return nil
	}

	hours := make([]time.Time, 0, len(hourSet))
	for h := range hourSet {
		hours = append(hours, h)
	}
	sort.Slice(hours, func(i, j int) bool { return hours[i].Before(hours[j]) })

	// Group samples into per-series ordered lists (distinct hours per series).
	series := make(map[seriesKey][]sample)
	for k, s := range latest {
		sk := seriesKey{org: k.org, variant: k.variant}
		series[sk] = append(series[sk], s)
	}
	for sk := range series {
		ss := series[sk]
		sort.Slice(ss, func(i, j int) bool { return ss[i].hour.Before(ss[j].hour) })
		series[sk] = ss
	}

	// Phase 2: forward-fill each series across the global hour axis.
	out := make(map[fillBucketKey][]P)
	for sk, samples := range series {
		idx := -1 // index of the series' most recent sample at or before the hour
		for _, h := range hours {
			for idx+1 < len(samples) && !samples[idx+1].hour.After(h) {
				idx++
			}
			if idx < 0 {
				continue // series has not started yet at this hour
			}
			bk := fillBucketKey{hour: h, variant: sk.variant}
			out[bk] = append(out[bk], samples[idx].point)
		}
	}
	return out
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

	contrib := forwardFillContributions(points,
		func(p versionDistTrendPoint) (time.Time, bool) {
			t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
			return t, err == nil
		},
		func(p versionDistTrendPoint) string { return p.OrganisationName },
		func(versionDistTrendPoint) string { return "" },
	)

	result := make([]versionDistTrendPoint, 0, len(contrib))
	for bk, pts := range contrib {
		merged := versionDistTrendPoint{
			CompletedAt:  bk.hour.Format(trendTimestampFormat),
			Distribution: make(map[string]int),
		}
		for _, p := range pts {
			merged.TotalNodes += p.TotalNodes
			for ver, cnt := range p.Distribution {
				merged.Distribution[ver] += cnt
			}
		}
		result = append(result, merged)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CompletedAt < result[j].CompletedAt })
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

	contrib := forwardFillContributions(points,
		func(p staleTrendPoint) (time.Time, bool) {
			t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
			return t, err == nil
		},
		func(p staleTrendPoint) string { return p.OrganisationName },
		func(staleTrendPoint) string { return "" },
	)

	result := make([]staleTrendPoint, 0, len(contrib))
	for bk, pts := range contrib {
		merged := staleTrendPoint{CompletedAt: bk.hour.Format(trendTimestampFormat)}
		for _, p := range pts {
			merged.TotalNodes += p.TotalNodes
			merged.StaleNodes += p.StaleNodes
			merged.FreshNodes += p.FreshNodes
			merged.WarningNodes += p.WarningNodes
			merged.CriticalNodes += p.CriticalNodes
		}
		result = append(result, merged)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CompletedAt < result[j].CompletedAt })
	return result
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

	contrib := forwardFillContributions(points,
		func(p complexityTrendPoint) (time.Time, bool) {
			t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
			return t, err == nil
		},
		func(p complexityTrendPoint) string { return p.OrganisationName },
		func(p complexityTrendPoint) string { return p.TargetChefVersion },
	)

	result := make([]complexityTrendPoint, 0, len(contrib))
	for bk, pts := range contrib {
		merged := complexityTrendPoint{
			CompletedAt:       bk.hour.Format(trendTimestampFormat),
			TargetChefVersion: bk.variant,
		}
		for _, p := range pts {
			merged.TotalCookbooks += p.TotalCookbooks
			merged.TotalScore += p.TotalScore
			merged.LowCount += p.LowCount
			merged.MediumCount += p.MediumCount
			merged.HighCount += p.HighCount
			merged.CriticalCount += p.CriticalCount
		}
		if merged.TotalCookbooks > 0 {
			merged.AverageScore = float64(merged.TotalScore) / float64(merged.TotalCookbooks)
		}
		result = append(result, merged)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CompletedAt != result[j].CompletedAt {
			return result[i].CompletedAt < result[j].CompletedAt
		}
		return result[i].TargetChefVersion < result[j].TargetChefVersion
	})
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

	contrib := forwardFillContributions(points,
		func(p readinessTrendPoint) (time.Time, bool) {
			t, err := time.Parse(trendTimestampFormat, p.CompletedAt)
			return t, err == nil
		},
		func(p readinessTrendPoint) string { return p.OrganisationName },
		func(p readinessTrendPoint) string { return p.TargetChefVersion },
	)

	result := make([]readinessTrendPoint, 0, len(contrib))
	for bk, pts := range contrib {
		merged := readinessTrendPoint{
			CompletedAt:       bk.hour.Format(trendTimestampFormat),
			TargetChefVersion: bk.variant,
		}
		for _, p := range pts {
			merged.TotalNodes += p.TotalNodes
			merged.ReadyNodes += p.ReadyNodes
			merged.NeedsReviewNodes += p.NeedsReviewNodes
			merged.BlockedNodes += p.BlockedNodes
		}
		if merged.TotalNodes > 0 {
			merged.ReadyPercent = float64(merged.ReadyNodes) / float64(merged.TotalNodes) * 100
		}
		result = append(result, merged)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CompletedAt != result[j].CompletedAt {
			return result[i].CompletedAt < result[j].CompletedAt
		}
		return result[i].TargetChefVersion < result[j].TargetChefVersion
	})
	return result
}
