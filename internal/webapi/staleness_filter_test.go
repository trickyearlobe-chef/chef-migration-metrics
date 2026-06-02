// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseStalenessFilter_Default(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	sf := parseStalenessFilter(req)
	if !sf.fresh || !sf.warning || !sf.critical {
		t.Error("default should include all tiers")
	}
	if sf.explicit {
		t.Error("default should not be explicit")
	}
	if !sf.isDefault() {
		t.Error("should be default")
	}
}

func TestParseStalenessFilter_FreshOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?stale=fresh", nil)
	sf := parseStalenessFilter(req)
	if !sf.fresh {
		t.Error("should include fresh")
	}
	if sf.warning || sf.critical {
		t.Error("should not include warning or critical")
	}
	if !sf.explicit {
		t.Error("should be explicit")
	}
	if !sf.isFreshOnly() {
		t.Error("should be fresh-only")
	}
}

func TestParseStalenessFilter_AllTiers(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?stale=fresh,warning,critical", nil)
	sf := parseStalenessFilter(req)
	if !sf.fresh || !sf.warning || !sf.critical {
		t.Errorf("should include all tiers: fresh=%v warning=%v critical=%v", sf.fresh, sf.warning, sf.critical)
	}
	if sf.isDefault() {
		t.Error("should not be default when all tiers are specified")
	}
}

func TestParseStalenessFilter_WarningCriticalOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?stale=warning,critical", nil)
	sf := parseStalenessFilter(req)
	if sf.fresh {
		t.Error("should not include fresh")
	}
	if !sf.warning || !sf.critical {
		t.Error("should include warning and critical")
	}
	if !sf.includesNonFresh() {
		t.Error("should include non-fresh")
	}
	if sf.includesFresh() {
		t.Error("should not include fresh")
	}
}

func TestParseStalenessFilter_InvalidFallsToFresh(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?stale=bogus", nil)
	sf := parseStalenessFilter(req)
	if !sf.fresh {
		t.Error("invalid input should default to fresh")
	}
	if !sf.explicit {
		t.Error("should be explicit since param was present")
	}
}

func TestParseStalenessFilter_CaseInsensitive(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?stale=Fresh,WARNING", nil)
	sf := parseStalenessFilter(req)
	if !sf.fresh || !sf.warning {
		t.Error("should be case insensitive")
	}
}
