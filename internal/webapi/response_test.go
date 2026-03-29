// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import "testing"

func TestPaginateSlice(t *testing.T) {
	items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	tests := []struct {
		name      string
		items     []int
		page      int
		perPage   int
		wantPage  []int
		wantTotal int
	}{
		{
			name:      "first page",
			items:     items,
			page:      1,
			perPage:   3,
			wantPage:  []int{0, 1, 2},
			wantTotal: 10,
		},
		{
			name:      "middle page",
			items:     items,
			page:      2,
			perPage:   3,
			wantPage:  []int{3, 4, 5},
			wantTotal: 10,
		},
		{
			name:      "last partial page",
			items:     items,
			page:      4,
			perPage:   3,
			wantPage:  []int{9},
			wantTotal: 10,
		},
		{
			name:      "page beyond end",
			items:     items,
			page:      100,
			perPage:   3,
			wantPage:  []int{},
			wantTotal: 10,
		},
		{
			name:      "empty slice",
			items:     []int{},
			page:      1,
			perPage:   10,
			wantPage:  []int{},
			wantTotal: 0,
		},
		{
			name:      "nil slice",
			items:     nil,
			page:      1,
			perPage:   5,
			wantPage:  nil,
			wantTotal: 0,
		},
		{
			name:      "exact fit single page",
			items:     items[:5],
			page:      1,
			perPage:   5,
			wantPage:  []int{0, 1, 2, 3, 4},
			wantTotal: 5,
		},
		{
			name:      "per_page larger than slice",
			items:     items[:3],
			page:      1,
			perPage:   50,
			wantPage:  []int{0, 1, 2},
			wantTotal: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pg := PaginationParams{Page: tt.page, PerPage: tt.perPage}
			gotPage, gotTotal := PaginateSlice(tt.items, pg)

			if gotTotal != tt.wantTotal {
				t.Errorf("total = %d, want %d", gotTotal, tt.wantTotal)
			}

			if len(gotPage) != len(tt.wantPage) {
				t.Fatalf("page length = %d, want %d", len(gotPage), len(tt.wantPage))
			}

			for i := range gotPage {
				if gotPage[i] != tt.wantPage[i] {
					t.Errorf("page[%d] = %d, want %d", i, gotPage[i], tt.wantPage[i])
				}
			}
		})
	}
}
