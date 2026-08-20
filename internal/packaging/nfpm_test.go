// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	repoRoot     = "../.."
	nfpmFile     = repoRoot + "/nfpm.yaml"
	makefileFile = repoRoot + "/Makefile"
	packagedBin  = "/usr/bin/chef-migration-metrics"
)

type nfpmConfig struct {
	Platform string `yaml:"platform"`
	Contents []struct {
		Src string `yaml:"src"`
		Dst string `yaml:"dst"`
	} `yaml:"contents"`
}

func readNfpm(t *testing.T, path string) nfpmConfig {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var cfg nfpmConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return cfg
}

// binarySrc is the source path nfpm packages as the service binary.
func (c nfpmConfig) binarySrc(t *testing.T) string {
	t.Helper()
	for _, e := range c.Contents {
		if e.Dst == packagedBin {
			return e.Src
		}
	}
	t.Fatalf("no contents entry installs %s", packagedBin)
	return ""
}

func readMakefile(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(makefileFile)
	if err != nil {
		t.Fatalf("read %s: %v", makefileFile, err)
	}
	return string(b)
}

// rules returns the prerequisites and recipe of every rule whose target
// matches pattern, keyed by the pattern's first submatch.
func rules(t *testing.T, mk, pattern string) map[string][2]string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^` + pattern + `:([^\n#]*)(?:#[^\n]*)?\n((?:[\t ].*\n)*)`)
	out := map[string][2]string{}
	for _, m := range re.FindAllStringSubmatch(mk, -1) {
		out[m[1]] = [2]string{m[2], m[3]}
	}
	return out
}

func keys(m map[string][2]string) []string {
	var k []string
	for s := range m {
		k = append(k, s)
	}
	sort.Strings(k)
	return k
}

// An RPM or DEB is a Linux package however it was built. Its binary must come
// from a cross-compile: on Windows the host build is a .exe (which nfpm cannot
// find under the name it expects), and on macOS it is a Mach-O binary that
// would be packaged with no complaint at all.
func TestPackagedBinaryComesFromALinuxCrossBuild(t *testing.T) {
	cfg := readNfpm(t, nfpmFile)

	if cfg.Platform != "linux" {
		t.Fatalf("platform = %q, want linux — this test assumes the package targets Linux", cfg.Platform)
	}

	src := cfg.binarySrc(t)
	want := regexp.MustCompile(`^\./build/chef-migration-metrics-linux-\w+$`)
	if !want.MatchString(src) {
		t.Errorf("binary src = %q, want a linux cross-compile output matching %v — the host build is the wrong platform when packaging from Windows or macOS", src, want)
	}
}

// Every architecture we cross-compile for Linux is an architecture somebody can
// be handed a package for. Shipping amd64 only because that is what the build
// host happened to be is the fault this guards.
func TestEveryLinuxCrossBuildHasAnRpmAndADeb(t *testing.T) {
	mk := readMakefile(t)

	built := keys(rules(t, mk, `build-linux-(\w+)`))
	if len(built) < 2 {
		t.Fatalf("found linux cross-compile targets for %v, expected more than one architecture", built)
	}

	for _, format := range []string{"rpm", "deb"} {
		packaged := keys(rules(t, mk, `package-`+format+`-(\w+)`))
		if strings.Join(packaged, ",") != strings.Join(built, ",") {
			t.Errorf("%s targets cover %v but linux cross-compiles cover %v", format, packaged, built)
		}

		aggregate, ok := rules(t, mk, `(package-`+format+`)`)["package-"+format]
		if !ok {
			t.Fatalf("Makefile has no package-%s rule", format)
		}
		for _, arch := range built {
			want := "package-" + format + "-" + arch
			if !strings.Contains(aggregate[0], want) {
				t.Errorf("package-%s does not depend on %s, so it ships only some architectures", format, want)
			}
		}
	}
}

// Each package target must build the binary it packages, and must package the
// architecture it names. Depending on the host `build` target packages a binary
// for whatever machine ran make.
func TestPackageTargetsBuildTheArchitectureTheyName(t *testing.T) {
	mk := readMakefile(t)

	for _, format := range []string{"rpm", "deb"} {
		for arch, rule := range rules(t, mk, `package-`+format+`-(\w+)`) {
			target := "package-" + format + "-" + arch
			prereqs, recipe := strings.Fields(rule[0]), rule[1]

			for _, p := range prereqs {
				if p == "build" {
					t.Errorf("%s depends on the host build target — that is a Windows .exe or a macOS binary when packaging off Linux", target)
				}
			}
			if want := "build-linux-" + arch; !contains(prereqs, want) {
				t.Errorf("%s prerequisites %v do not include %s", target, prereqs, want)
			}
			if want := "ARCH=" + arch; !strings.Contains(recipe, want) {
				t.Errorf("%s recipe does not set %s, so the package would be labelled with another architecture", target, want)
			}
			if want := "nfpm-" + arch + ".yaml"; !strings.Contains(recipe, want) {
				t.Errorf("%s recipe does not package with %s, so it would carry another architecture's binary", target, want)
			}
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// The per-architecture configs are derived from nfpm.yaml by the Makefile, so
// the derivation is the thing that has to be right: generate them and read back
// which binary each one would package.
func TestGeneratedConfigPackagesItsOwnArchitecture(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not on PATH")
	}
	mk := readMakefile(t)

	for _, arch := range keys(rules(t, mk, `build-linux-(\w+)`)) {
		generated := filepath.Join("build", "nfpm-"+arch+".yaml")
		// Remove it first: make would otherwise report an existing file as up
		// to date and this test would read the previous run's output.
		if err := os.Remove(filepath.Join(repoRoot, generated)); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s: %v", generated, err)
		}
		out, err := exec.Command("make", "-C", repoRoot, generated).CombinedOutput()
		if err != nil {
			t.Fatalf("make %s: %v\n%s", generated, err, out)
		}

		src := readNfpm(t, filepath.Join(repoRoot, generated)).binarySrc(t)
		if want := "./build/chef-migration-metrics-linux-" + arch; src != want {
			t.Errorf("%s packages %q, want %q", generated, src, want)
		}
	}
}

// Git Bash on Windows ships no zip(1). Archive creation must therefore not
// depend on it, or `make package-archives` fails on the one platform whose
// archive is a zip in the first place.
func TestWindowsArchivesDoNotRequireZip(t *testing.T) {
	mk := readMakefile(t)

	def := regexp.MustCompile(`(?s)define make_zip\n(.*?)\nendef`).FindStringSubmatch(mk)
	if def == nil {
		t.Fatal("Makefile has no make_zip definition")
	}
	body := def[1]
	if !strings.Contains(body, "command -v zip") {
		t.Error("make_zip does not check whether zip is available before using it")
	}
	var alternatives int
	for _, alt := range []string{"bsdtar", "Compress-Archive"} {
		if strings.Contains(body, alt) {
			alternatives++
		}
	}
	if alternatives == 0 {
		t.Error("make_zip offers no alternative to zip for hosts without it")
	}

	archives := regexp.MustCompile(`(?ms)^package-archives:.*?\n\n`).FindString(mk)
	if archives == "" {
		t.Fatal("Makefile has no package-archives rule")
	}
	if bare := regexp.MustCompile(`(?m)^\s*.*[^v] zip -`).FindString(archives); bare != "" {
		t.Errorf("package-archives calls zip directly: %s", strings.TrimSpace(bare))
	}
}
