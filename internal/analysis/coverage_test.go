// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseKitchenPlatformName_Ubuntu(t *testing.T) {
	os, version, ok := ParseKitchenPlatformName("ubuntu-22.04")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if os != "ubuntu" {
		t.Errorf("os: got %q, want %q", os, "ubuntu")
	}
	if version != "22.04" {
		t.Errorf("version: got %q, want %q", version, "22.04")
	}
}

func TestParseKitchenPlatformName_CentOS7(t *testing.T) {
	os, version, ok := ParseKitchenPlatformName("centos-7")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if os != "centos" {
		t.Errorf("os: got %q, want %q", os, "centos")
	}
	if version != "7" {
		t.Errorf("version: got %q, want %q", version, "7")
	}
}

func TestParseKitchenPlatformName_Windows(t *testing.T) {
	os, version, ok := ParseKitchenPlatformName("windows-2022")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if os != "windows" {
		t.Errorf("os: got %q, want %q", os, "windows")
	}
	if version != "2022" {
		t.Errorf("version: got %q, want %q", version, "2022")
	}
}

func TestParseKitchenPlatformName_NoHyphen(t *testing.T) {
	_, _, ok := ParseKitchenPlatformName("default")
	if ok {
		t.Fatal("expected ok=false for name without hyphen")
	}
}

func TestParseKitchenPlatformName_HyphenAtEnd(t *testing.T) {
	_, _, ok := ParseKitchenPlatformName("ubuntu-")
	if ok {
		t.Fatal("expected ok=false for trailing hyphen")
	}
}

func TestParseKitchenPlatformName_HyphenAtStart(t *testing.T) {
	_, _, ok := ParseKitchenPlatformName("-22.04")
	if ok {
		t.Fatal("expected ok=false for leading hyphen")
	}
}

func TestParseKitchenPlatformName_MultipleHyphens(t *testing.T) {
	os, version, ok := ParseKitchenPlatformName("rocky-linux-9")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if os != "rocky-linux" {
		t.Errorf("os: got %q, want %q", os, "rocky-linux")
	}
	if version != "9" {
		t.Errorf("version: got %q, want %q", version, "9")
	}
}

func TestMajorVersionMatch_Exact(t *testing.T) {
	if !majorVersionMatch("7", "7.9.2009") {
		t.Error("expected match: kitchen=7, prod=7.9.2009")
	}
}

func TestMajorVersionMatch_DottedKitchen(t *testing.T) {
	if !majorVersionMatch("22.04", "22.04.1") {
		t.Error("expected match: kitchen=22.04, prod=22.04.1")
	}
}

func TestMajorVersionMatch_DottedKitchenNoMatch(t *testing.T) {
	if majorVersionMatch("22.04", "22.10") {
		t.Error("expected no match: kitchen=22.04, prod=22.10 (different Ubuntu release)")
	}
}

func TestMajorVersionMatch_DottedKitchenExact(t *testing.T) {
	if !majorVersionMatch("22.04", "22.04") {
		t.Error("expected match: kitchen=22.04, prod=22.04 (exact)")
	}
}

func TestMajorVersionMatch_NoMatch(t *testing.T) {
	if majorVersionMatch("8", "7.9.2009") {
		t.Error("expected no match: kitchen=8, prod=7.9.2009")
	}
}

func TestMajorVersionMatch_Empty(t *testing.T) {
	if majorVersionMatch("", "7.9") {
		t.Error("expected no match when kitchen version is empty")
	}
	if majorVersionMatch("7", "") {
		t.Error("expected no match when production version is empty")
	}
}

func TestComputeCoverage_FullMatch(t *testing.T) {
	kitchen := []string{"ubuntu-22.04", "centos-7"}
	prod := []ProductionPlatform{
		{Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian", NodeCount: 47},
		{Platform: "centos", PlatformVersion: "7.9.2009", PlatformFamily: "rhel", NodeCount: 12},
	}

	r := ComputeCoverage(kitchen, prod)

	if r.GapCount != 0 {
		t.Errorf("gap_count: got %d, want 0", r.GapCount)
	}
	if r.CoveragePercentage != 100.0 {
		t.Errorf("coverage: got %.1f, want 100.0", r.CoveragePercentage)
	}
	if r.TotalProductionNodes != 59 {
		t.Errorf("total_production_nodes: got %d, want 59", r.TotalProductionNodes)
	}
	if r.CoveredNodeCount != 59 {
		t.Errorf("covered_node_count: got %d, want 59", r.CoveredNodeCount)
	}
	if len(r.TestedAndInProd) != 2 {
		t.Errorf("tested_and_in_production length: got %d, want 2", len(r.TestedAndInProd))
	}
	if len(r.TestedNotInProd) != 0 {
		t.Errorf("tested_not_in_production length: got %d, want 0", len(r.TestedNotInProd))
	}
	if len(r.InProdNotTested) != 0 {
		t.Errorf("in_production_not_tested length: got %d, want 0", len(r.InProdNotTested))
	}
}

func TestComputeCoverage_WithGaps(t *testing.T) {
	kitchen := []string{"ubuntu-22.04"}
	prod := []ProductionPlatform{
		{Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian", NodeCount: 47},
		{Platform: "rocky", PlatformVersion: "9.3", PlatformFamily: "rhel", NodeCount: 8},
	}

	r := ComputeCoverage(kitchen, prod)

	if r.GapCount != 1 {
		t.Errorf("gap_count: got %d, want 1", r.GapCount)
	}
	if r.TotalProductionNodes != 55 {
		t.Errorf("total_production_nodes: got %d, want 55", r.TotalProductionNodes)
	}
	if r.CoveredNodeCount != 47 {
		t.Errorf("covered_node_count: got %d, want 47", r.CoveredNodeCount)
	}
	if r.CoveragePercentage != 85.5 {
		t.Errorf("coverage: got %.1f, want 85.5", r.CoveragePercentage)
	}
	if len(r.InProdNotTested) != 1 {
		t.Fatalf("in_production_not_tested length: got %d, want 1", len(r.InProdNotTested))
	}
	if r.InProdNotTested[0].Platform != "rocky" {
		t.Errorf("gap platform: got %q, want %q", r.InProdNotTested[0].Platform, "rocky")
	}
}

func TestComputeCoverage_FamilyMatch(t *testing.T) {
	kitchen := []string{"rhel-9"}
	prod := []ProductionPlatform{
		{Platform: "rocky", PlatformVersion: "9.3", PlatformFamily: "rhel", NodeCount: 8},
		{Platform: "alma", PlatformVersion: "9.1", PlatformFamily: "rhel", NodeCount: 5},
	}

	r := ComputeCoverage(kitchen, prod)

	if r.GapCount != 0 {
		t.Errorf("gap_count: got %d, want 0", r.GapCount)
	}
	if r.CoveredNodeCount != 13 {
		t.Errorf("covered_node_count: got %d, want 13", r.CoveredNodeCount)
	}
	if r.CoveragePercentage != 100.0 {
		t.Errorf("coverage: got %.1f, want 100.0", r.CoveragePercentage)
	}
	if len(r.TestedAndInProd) != 2 {
		t.Errorf("tested_and_in_production length: got %d, want 2", len(r.TestedAndInProd))
	}
}

func TestComputeCoverage_MajorVersionMatch(t *testing.T) {
	kitchen := []string{"centos-7"}
	prod := []ProductionPlatform{
		{Platform: "centos", PlatformVersion: "7.9.2009", PlatformFamily: "rhel", NodeCount: 12},
	}

	r := ComputeCoverage(kitchen, prod)

	if r.GapCount != 0 {
		t.Errorf("gap_count: got %d, want 0", r.GapCount)
	}
	if r.CoveredNodeCount != 12 {
		t.Errorf("covered_node_count: got %d, want 12", r.CoveredNodeCount)
	}
	if len(r.TestedAndInProd) != 1 {
		t.Fatalf("tested_and_in_production length: got %d, want 1", len(r.TestedAndInProd))
	}
	if r.TestedAndInProd[0].KitchenName != "centos-7" {
		t.Errorf("kitchen_name: got %q, want %q", r.TestedAndInProd[0].KitchenName, "centos-7")
	}
}

func TestComputeCoverage_UnparseableName(t *testing.T) {
	kitchen := []string{"default"}
	prod := []ProductionPlatform{
		{Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian", NodeCount: 10},
	}

	r := ComputeCoverage(kitchen, prod)

	if len(r.TestedNotInProd) != 1 {
		t.Fatalf("tested_not_in_production length: got %d, want 1", len(r.TestedNotInProd))
	}
	if r.TestedNotInProd[0] != "default" {
		t.Errorf("tested_not_in_production[0]: got %q, want %q", r.TestedNotInProd[0], "default")
	}
	if r.GapCount != 1 {
		t.Errorf("gap_count: got %d, want 1", r.GapCount)
	}
	if len(r.InProdNotTested) != 1 {
		t.Fatalf("in_production_not_tested length: got %d, want 1", len(r.InProdNotTested))
	}
	if r.InProdNotTested[0].Platform != "ubuntu" {
		t.Errorf("gap platform: got %q, want %q", r.InProdNotTested[0].Platform, "ubuntu")
	}
}

func TestComputeCoverage_Empty(t *testing.T) {
	r := ComputeCoverage(nil, nil)

	if r.GapCount != 0 {
		t.Errorf("gap_count: got %d, want 0", r.GapCount)
	}
	if r.TotalProductionNodes != 0 {
		t.Errorf("total_production_nodes: got %d, want 0", r.TotalProductionNodes)
	}
	if r.CoveragePercentage != 0 {
		t.Errorf("coverage: got %.1f, want 0.0", r.CoveragePercentage)
	}
	if r.KitchenPlatforms == nil {
		t.Error("kitchen_platforms should not be nil")
	}
	if r.ProductionPlatforms == nil {
		t.Error("production_platforms should not be nil")
	}
	if r.TestedAndInProd == nil {
		t.Error("tested_and_in_production should not be nil")
	}
	if r.TestedNotInProd == nil {
		t.Error("tested_not_in_production should not be nil")
	}
	if r.InProdNotTested == nil {
		t.Error("in_production_not_tested should not be nil")
	}
}

func TestComputeCoverage_TestedNotInProd(t *testing.T) {
	kitchen := []string{"ubuntu-22.04", "centos-7"}
	prod := []ProductionPlatform{
		{Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian", NodeCount: 10},
	}

	r := ComputeCoverage(kitchen, prod)

	if len(r.TestedNotInProd) != 1 {
		t.Fatalf("tested_not_in_production length: got %d, want 1", len(r.TestedNotInProd))
	}
	if r.TestedNotInProd[0] != "centos-7" {
		t.Errorf("tested_not_in_production[0]: got %q, want %q", r.TestedNotInProd[0], "centos-7")
	}
	if r.CoveredNodeCount != 10 {
		t.Errorf("covered_node_count: got %d, want 10", r.CoveredNodeCount)
	}
	if r.GapCount != 0 {
		t.Errorf("gap_count: got %d, want 0", r.GapCount)
	}
}

func TestComputeCoverage_CoveragePercentage(t *testing.T) {
	kitchen := []string{"ubuntu-22.04", "centos-7"}
	prod := []ProductionPlatform{
		{Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian", NodeCount: 47},
		{Platform: "centos", PlatformVersion: "7.9.2009", PlatformFamily: "rhel", NodeCount: 12},
		{Platform: "rocky", PlatformVersion: "9.3", PlatformFamily: "rhel", NodeCount: 8},
	}

	r := ComputeCoverage(kitchen, prod)

	// 59 covered out of 67 total = 88.05970... rounds to 88.1
	if r.TotalProductionNodes != 67 {
		t.Errorf("total_production_nodes: got %d, want 67", r.TotalProductionNodes)
	}
	if r.CoveredNodeCount != 59 {
		t.Errorf("covered_node_count: got %d, want 59", r.CoveredNodeCount)
	}
	if r.CoveragePercentage != 88.1 {
		t.Errorf("coverage: got %.1f, want 88.1", r.CoveragePercentage)
	}
}

func TestParseKitchenYMLPlatforms_Standard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".kitchen.yml")
	content := `---
driver:
  name: vagrant

provisioner:
  name: chef_zero

platforms:
  - name: ubuntu-22.04
  - name: centos-7
  - name: rocky-linux-9

suites:
  - name: default
    run_list:
      - recipe[mycookbook::default]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	platforms := ParseKitchenYMLPlatforms(path)

	if len(platforms) != 3 {
		t.Fatalf("platforms length: got %d, want 3", len(platforms))
	}
	expected := []string{"ubuntu-22.04", "centos-7", "rocky-linux-9"}
	for i, want := range expected {
		if platforms[i] != want {
			t.Errorf("platforms[%d]: got %q, want %q", i, platforms[i], want)
		}
	}
}

func TestParseKitchenYMLPlatforms_NoPlatforms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".kitchen.yml")
	content := `---
driver:
  name: vagrant

suites:
  - name: default
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	platforms := ParseKitchenYMLPlatforms(path)

	if platforms != nil {
		t.Errorf("expected nil, got %v", platforms)
	}
}

func TestParseKitchenYMLPlatforms_FileNotFound(t *testing.T) {
	platforms := ParseKitchenYMLPlatforms("/nonexistent/path/.kitchen.yml")

	if platforms != nil {
		t.Errorf("expected nil, got %v", platforms)
	}
}

func TestParseKitchenYMLPlatforms_InlineComments(t *testing.T) {
	content := "platforms:\n  - name: ubuntu-22.04  # LTS\n  - name: centos-7 # old\n"
	dir := t.TempDir()
	path := filepath.Join(dir, ".kitchen.yml")
	os.WriteFile(path, []byte(content), 0644)

	got := ParseKitchenYMLPlatforms(path)
	want := []string{"ubuntu-22.04", "centos-7"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseKitchenYMLPlatforms_QuotedNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".kitchen.yml")
	content := `---
platforms:
  - name: "ubuntu-22.04"
  - name: 'centos-7'
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	platforms := ParseKitchenYMLPlatforms(path)

	if len(platforms) != 2 {
		t.Fatalf("platforms length: got %d, want 2", len(platforms))
	}
	if platforms[0] != "ubuntu-22.04" {
		t.Errorf("platforms[0]: got %q, want %q", platforms[0], "ubuntu-22.04")
	}
	if platforms[1] != "centos-7" {
		t.Errorf("platforms[1]: got %q, want %q", platforms[1], "centos-7")
	}
}
