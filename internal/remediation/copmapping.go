// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package remediation provides auto-correct preview generation,
// cop-to-documentation mapping, and cookbook complexity scoring for the
// Chef migration analysis pipeline.
package remediation

import "strings"

// CopMapping describes a single CookStyle cop's migration documentation.
// Each entry maps a cop_name to its human-readable description, migration
// URL, version lifecycle information, and a brief code example showing the
// old pattern and its replacement.
type CopMapping struct {
	// CopName is the fully qualified cop name (e.g. "Chef/Deprecations/ResourceWithoutUnifiedTrue").
	CopName string `json:"cop_name"`

	// Description is a human-readable explanation of the deprecation or
	// correctness issue and what to change.
	Description string `json:"description"`

	// MigrationURL is a link to the relevant Chef migration documentation.
	MigrationURL string `json:"migration_url"`

	// IntroducedIn is the Chef Client version where the deprecation warning
	// was first emitted, or the correctness check was first enforced.
	IntroducedIn string `json:"introduced_in"`

	// RemovedIn is the Chef Client version where the deprecated feature was
	// removed. Empty if the feature has not yet been removed.
	RemovedIn string `json:"removed_in"`

	// ReplacementPattern is a brief code example showing the old pattern
	// and the new pattern, separated by a blank line or comment.
	ReplacementPattern string `json:"replacement_pattern"`
}

// EnrichedOffense is a CookStyle offense augmented with its migration
// documentation from the cop mapping table. The analysis pipeline attaches
// this to each offense before persisting CookStyle results.
type EnrichedOffense struct {
	CopName  string          `json:"cop_name"`
	Severity string          `json:"severity"`
	Message  string          `json:"message"`
	Location OffenseLocation `json:"location"`

	// Remediation is the migration documentation for this cop, or nil if
	// no mapping exists.
	Remediation *CopMapping `json:"remediation,omitempty"`
}

// OffenseLocation mirrors the location fields from CookStyle JSON output.
type OffenseLocation struct {
	File        string `json:"file,omitempty"`
	StartLine   int    `json:"start_line"`
	StartColumn int    `json:"start_column"`
	LastLine    int    `json:"last_line"`
	LastColumn  int    `json:"last_column"`
}

// copMappingIndex is the in-memory lookup table keyed by cop_name.
// Built once at init time from the embedded mapping data.
var copMappingIndex map[string]*CopMapping

func init() {
	copMappingIndex = make(map[string]*CopMapping, len(embeddedCopMappings))
	for i := range embeddedCopMappings {
		copMappingIndex[embeddedCopMappings[i].CopName] = &embeddedCopMappings[i]
	}
}

// LookupCop returns the migration documentation for the given cop name,
// or nil if no mapping exists.
func LookupCop(copName string) *CopMapping {
	return copMappingIndex[copName]
}

// copVariant is one message-discriminated variant of a poly-method cop: a
// single cop name (e.g. Lint/DeprecatedClassMethods) flags several unrelated
// deprecations whose only distinguishing signal is the offence message. Token
// is a substring of the offence message (the deprecated method name); Mapping
// is that variant's own migration documentation and RemovedIn.
type copVariant struct {
	Token   string
	Mapping CopMapping
}

// polyMethodCopVariants maps a poly-method cop name to its message-discriminated
// variants. See specifications/cop-classification.md (Poly-method cops). This
// table is the source of truth for per-message remediation + RemovedIn and is
// linter-guarded alongside the rest of the curated removal set.
var polyMethodCopVariants = map[string][]copVariant{
	"Lint/DeprecatedConstants": {
		{
			// Net::HTTPServerException is still defined on Ruby 3.4 (an alias of
			// Net::HTTPClientException) → Review, not Blocker. Lab-verified 2026-07-16
			// on CC19.3.15. Every other constant this cop flags by default was removed
			// in Ruby 3.x and falls through to the base Blocker mapping.
			Token: "Net::HTTPServerException",
			Mapping: CopMapping{
				CopName:      "Lint/DeprecatedConstants",
				Description:  "Net::HTTPServerException is deprecated (still present on Ruby 3.4 as an alias). Use Net::HTTPClientException.",
				MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedconstants",
				RemovedIn:    "",
				ReplacementPattern: `# Before:
rescue Net::HTTPServerException

# After:
rescue Net::HTTPClientException`,
			},
		},
	},
	"Lint/DeprecatedClassMethods": {
		{
			// Removed in Ruby 3.4 (bundled with Chef 19) → Blocker. Lab-verified.
			Token: "File.exists?",
			Mapping: CopMapping{
				CopName:      "Lint/DeprecatedClassMethods",
				Description:  "File.exists? was removed in Ruby 3.4 (bundled with Chef Infra Client 19). Calls raise NoMethodError at converge time. Use File.exist?.",
				MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedclassmethods",
				RemovedIn:    "19.0",
				ReplacementPattern: `# Before:
File.exists?(path)

# After:
File.exist?(path)`,
			},
		},
		{
			// Removed in Ruby 3.4 (bundled with Chef 19) → Blocker. Lab-verified.
			Token: "Dir.exists?",
			Mapping: CopMapping{
				CopName:      "Lint/DeprecatedClassMethods",
				Description:  "Dir.exists? was removed in Ruby 3.4 (bundled with Chef Infra Client 19). Calls raise NoMethodError at converge time. Use Dir.exist?.",
				MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedclassmethods",
				RemovedIn:    "19.0",
				ReplacementPattern: `# Before:
Dir.exists?(path)

# After:
Dir.exist?(path)`,
			},
		},
		{
			// Deprecated only (not removed) → Review (no RemovedIn).
			Token: "Socket.gethostbyname",
			Mapping: CopMapping{
				CopName:      "Lint/DeprecatedClassMethods",
				Description:  "Socket.gethostbyname is deprecated (not removed) and does not support IPv6. Use Addrinfo.getaddrinfo.",
				MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedclassmethods",
				RemovedIn:    "",
				ReplacementPattern: `# Before:
Socket.gethostbyname('example.com')

# After:
Addrinfo.getaddrinfo('example.com', nil)`,
			},
		},
		{
			// Deprecated only (not removed) → Review (no RemovedIn).
			Token: "Socket.gethostbyaddr",
			Mapping: CopMapping{
				CopName:      "Lint/DeprecatedClassMethods",
				Description:  "Socket.gethostbyaddr is deprecated (not removed) and does not support IPv6. Use Addrinfo#getnameinfo.",
				MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedclassmethods",
				RemovedIn:    "",
				ReplacementPattern: `# Before:
Socket.gethostbyaddr(addr)

# After:
Addrinfo.tcp(ip, port).getnameinfo`,
			},
		},
		{
			// ENV.clone/ENV.dup raise TypeError on Ruby 3 (Chef 19) → Blocker.
			// Lab-verified 2026-07-16 on CC19.3.15 (Ruby 3.4.8).
			Token: "ENV.clone",
			Mapping: CopMapping{
				CopName:      "Lint/DeprecatedClassMethods",
				Description:  "ENV.clone raises TypeError on Ruby 3 (bundled with Chef Infra Client 19). Use ENV.to_h to copy the environment.",
				MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedclassmethods",
				RemovedIn:    "19.0",
				ReplacementPattern: `# Before:
ENV.clone

# After:
ENV.to_h`,
			},
		},
		{
			Token: "ENV.dup",
			Mapping: CopMapping{
				CopName:      "Lint/DeprecatedClassMethods",
				Description:  "ENV.dup raises TypeError on Ruby 3 (bundled with Chef Infra Client 19). Use ENV.to_h to copy the environment.",
				MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedclassmethods",
				RemovedIn:    "19.0",
				ReplacementPattern: `# Before:
ENV.dup

# After:
ENV.to_h`,
			},
		},
		{
			Token: "ENV.freeze",
			Mapping: CopMapping{
				CopName:      "Lint/DeprecatedClassMethods",
				Description:  "ENV.freeze raises TypeError on Ruby 3 (bundled with Chef Infra Client 19); ENV cannot be frozen. Remove the freeze call.",
				MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedclassmethods",
				RemovedIn:    "19.0",
				ReplacementPattern: `# Before:
ENV.freeze

# After:
# Remove — ENV cannot be frozen on Ruby 3+`,
			},
		},
		{
			// Deprecated only, still present on Ruby 3.4 → Review (no RemovedIn).
			Token: "iterator?",
			Mapping: CopMapping{
				CopName:      "Lint/DeprecatedClassMethods",
				Description:  "iterator? is deprecated (still present on Ruby 3.4). Use block_given?.",
				MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedclassmethods",
				RemovedIn:    "",
				ReplacementPattern: `# Before:
iterator?

# After:
block_given?`,
			},
		},
		{
			// The 2-arg form attr(:name, true) is deprecated but still present → Review.
			Token: "attr",
			Mapping: CopMapping{
				CopName:      "Lint/DeprecatedClassMethods",
				Description:  "The 2-argument form attr(:name, true) is deprecated (still present on Ruby 3.4). Use attr_accessor.",
				MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedclassmethods",
				RemovedIn:    "",
				ReplacementPattern: `# Before:
attr :name, true

# After:
attr_accessor :name`,
			},
		},
	},
}

// variantSeparators strips Ruby namespace/qualifier separators so one variant
// token matches every namespace spelling of the same deprecated method: e.g.
// token "File.exists?" matches "File.exists?", "::File.exists?", AND
// "::File::exists?" (all seen in real cookstyle output). Without this the
// double-colon method form would miss its variant and fall through to the base
// cop mapping — a fragile accident that would re-introduce a false-positive
// Blocker for a non-removed method spelled with "::". Scoped to poly-method
// matching only.
var variantSeparators = strings.NewReplacer(":", "", ".", "")

// matchVariant returns the poly-method variant whose (separator-normalised)
// token appears in the (separator-normalised) offence message, or nil when
// copName is not a poly-method cop or no variant matches. First match wins.
func matchVariant(copName, message string) *copVariant {
	variants, ok := polyMethodCopVariants[copName]
	if !ok {
		return nil
	}
	msg := variantSeparators.Replace(message)
	for i := range variants {
		if strings.Contains(msg, variantSeparators.Replace(variants[i].Token)) {
			return &variants[i]
		}
	}
	return nil
}

// OffenseVariantToken returns the poly-method variant token matched by the
// offence message (e.g. "Socket.gethostbyname"), or "" when copName is not a
// poly-method cop or no variant matches. Callers group offences of the same cop
// by variant so a Blocker variant and a Review variant render (and section)
// separately. See specifications/cop-classification.md (Poly-method cops).
func OffenseVariantToken(copName, message string) string {
	if v := matchVariant(copName, message); v != nil {
		return v.Token
	}
	return ""
}

// LookupCopForOffense returns the migration documentation for a specific
// offence, discriminating poly-method cops by their message. For a poly-method
// cop the matching variant's mapping (with its own RemovedIn) is returned; a
// message matching no variant, or a non-poly cop, falls back to LookupCop.
// See specifications/cop-classification.md (Poly-method cops).
func LookupCopForOffense(copName, message string) *CopMapping {
	if v := matchVariant(copName, message); v != nil {
		m := v.Mapping
		return &m
	}
	return LookupCop(copName)
}

// AllCopMappings returns a copy of the full mapping table. This is useful
// for the web API to expose the complete mapping to the frontend.
func AllCopMappings() []CopMapping {
	result := make([]CopMapping, len(embeddedCopMappings))
	copy(result, embeddedCopMappings)
	return result
}

// CopMappingCount returns the number of cops in the mapping table.
func CopMappingCount() int {
	return len(embeddedCopMappings)
}

// ---------------------------------------------------------------------------
// Embedded mapping data
// ---------------------------------------------------------------------------
//
// This table covers the most common Chef/Deprecations/* and
// Chef/Correctness/* cops that practitioners encounter during Chef Client
// upgrade projects. The mapping is shipped as compiled Go data in the
// application binary and can be updated by releasing a new version.
//
// Sources:
//   - https://docs.chef.io/workstation/cookstyle/
//   - https://github.com/chef/cookstyle/tree/main/lib/rubocop/cop/chef
//   - https://docs.chef.io/deprecations/
//   - https://docs.chef.io/unified_mode/

var embeddedCopMappings = []CopMapping{
	// -----------------------------------------------------------------------
	// Generic Ruby removals (RuboCop Lint department) — verified removals that
	// crash on the target Chef Client's bundled Ruby.
	// -----------------------------------------------------------------------
	{
		// RemovedIn 19.0 verified against CC19.3.14 (Ruby 3.4.8) in the lab
		// 2026-07-16: File.exists?/Dir.exists? raise NoMethodError; present (with a
		// deprecation warning) on CC18/Ruby 3.1.
		CopName:      "Lint/DeprecatedClassMethods",
		Description:  "File.exists? and Dir.exists? were removed in Ruby 3.4 (bundled with Chef Infra Client 19). Calls raise NoMethodError at converge time. Use File.exist? / Dir.exist?.",
		MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedclassmethods",
		IntroducedIn: "",
		RemovedIn:    "19.0",
		ReplacementPattern: `# Before:
File.exists?(path)

# After:
File.exist?(path)`,
	},
	{
		// Lab-verified 2026-07-16 on CC19.3.15 (Ruby 3.4.8): BigDecimal.new raises
		// NoMethodError (removed in Ruby 2.7). cookstyle's description says only
		// "deprecated", but the call is a hard runtime failure on the target →
		// Blocker (false-negative sweep: was defaulting to Review). Enabled by
		// default in cookstyle.
		CopName:      "Lint/BigDecimalNew",
		Description:  "BigDecimal.new was removed in Ruby 2.7 (the target Chef Infra Client 19 bundles Ruby 3.4). Calls raise NoMethodError at converge time. Use the BigDecimal() conversion function.",
		MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintbigdecimalnew",
		IntroducedIn: "",
		RemovedIn:    "19.0",
		ReplacementPattern: `# Before:
BigDecimal.new('1.23')

# After:
BigDecimal('1.23')`,
	},
	{
		// Lab-verified 2026-07-16 on CC19.3.15 (Ruby 3.4.8): the Fixnum and Bignum
		// constants were removed in Ruby 3.2 and raise NameError. Both constants the
		// cop flags are gone → uniform Blocker (false-negative sweep). Enabled by
		// default in cookstyle.
		CopName:      "Lint/UnifiedInteger",
		Description:  "The Fixnum and Bignum constants were removed in Ruby 3.2 (the target Chef Infra Client 19 bundles Ruby 3.4). References raise NameError. Use Integer.",
		MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintunifiedinteger",
		IntroducedIn: "",
		RemovedIn:    "19.0",
		ReplacementPattern: `# Before:
value.is_a?(Fixnum)

# After:
value.is_a?(Integer)`,
	},
	{
		// Poly-method cop (see polyMethodCopVariants below). Lab-verified 2026-07-16
		// on Ruby 3.4.8: of the constants this cop flags by default (NIL, TRUE,
		// FALSE, Net::HTTPServerException, Random::DEFAULT, Struct::Group,
		// Struct::Passwd) all but Net::HTTPServerException were removed in Ruby 3.x
		// and raise NameError. Base mapping is therefore Blocker; the single present
		// constant (Net::HTTPServerException) is carved out to Review by variant.
		CopName:      "Lint/DeprecatedConstants",
		Description:  "Flags deprecated Ruby constants. NIL/TRUE/FALSE (removed Ruby 3.0), Random::DEFAULT (removed Ruby 3.0), and Struct::Group/Struct::Passwd (removed Ruby 3.x) raise NameError on the target Chef Infra Client 19 (Ruby 3.4). Use the lowercase literals / documented replacements.",
		MigrationURL: "https://docs.rubocop.org/rubocop/cops_lint.html#lintdeprecatedconstants",
		IntroducedIn: "",
		RemovedIn:    "19.0",
		ReplacementPattern: `# Before:
x = NIL
r = Random::DEFAULT

# After:
x = nil
r = Random.new`,
	},
	// -----------------------------------------------------------------------
	// Chef/Deprecations
	// -----------------------------------------------------------------------
	//
	// The eight entries below were added by the 2026-07-16 false-negative sweep
	// (see scripts/cop-validation/README.md): cops that were defaulting to Review
	// but flag a class/helper/method genuinely removed on CC19.3.15, confirmed by
	// behavioural probe (call + catch) on the lab box — a hidden blocker, the
	// dangerous direction per specifications/cop-classification.md.
	{
		// Lab-verified 2026-07-16: Chef::REST is undefined on CC19.3.15 (NameError).
		// cookstyle description states removal in Chef Infra Client 13.
		CopName:      "Chef/Deprecations/UsesChefRESTHelpers",
		Description:  "Chef::REST was removed in Chef Infra Client 13. Its helpers raise NameError at converge time. Use Chef::ServerAPI for requests to the Chef Infra Server.",
		MigrationURL: "https://docs.chef.io/workstation/cookstyle/chef_deprecations_useschefresthelpers",
		IntroducedIn: "12.0",
		RemovedIn:    "13.0",
		ReplacementPattern: `# Before:
Chef::REST.new(url).get_rest(path)

# After:
Chef::ServerAPI.new(url).get(path)`,
	},
	{
		// Lab-verified 2026-07-16: Chef::ShellOut is undefined on CC19.3.15
		// (NameError). cookstyle description states removal in Chef Infra Client 13.
		CopName:      "Chef/Deprecations/ChefShellout",
		Description:  "The Chef::ShellOut class was removed in Chef Infra Client 13. References raise NameError at converge time. Use Mixlib::ShellOut, which behaves identically.",
		MigrationURL: "https://docs.chef.io/workstation/cookstyle/chef_deprecations_chefshellout",
		IntroducedIn: "12.0",
		RemovedIn:    "13.0",
		ReplacementPattern: `# Before:
Chef::ShellOut.new('command').run_command

# After:
Mixlib::ShellOut.new('command').run_command`,
	},
	{
		// Lab-verified 2026-07-16: Chef::Mixin::Command / ::Language /
		// ::LanguageIncludeRecipe / ::RecipeDefinitionDSLCore are all undefined on
		// CC19.3.15 (NameError). cookstyle description states removal in Chef 14.
		CopName:      "Chef/Deprecations/UsesDeprecatedMixins",
		Description:  "Several Chef mixin modules (e.g. Chef::Mixin::Command, Chef::Mixin::Language, Chef::Mixin::LanguageIncludeRecipe) were removed in Chef Infra Client 14. Including them raises NameError at converge time. Use the current DSL helpers.",
		MigrationURL: "https://docs.chef.io/workstation/cookstyle/chef_deprecations_usesdeprecatedmixins",
		IntroducedIn: "12.0",
		RemovedIn:    "14.0",
		ReplacementPattern: `# Before:
include Chef::Mixin::LanguageIncludeRecipe

# After:
# Remove the include — include_recipe/shell_out are available directly.`,
	},
	{
		// Lab-verified 2026-07-16: Chef::Resource does not respond to dsl_name on
		// CC19.3.15. cookstyle description: "Using dsl_name causes failures in Chef
		// Infra Client 13 and later."
		CopName:      "Chef/Deprecations/ResourceUsesDslNameMethod",
		Description:  "The dsl_name method on resources causes failures in Chef Infra Client 13 and later. Use resource_name (with provides) instead.",
		MigrationURL: "https://docs.chef.io/workstation/cookstyle/chef_deprecations_resourceusesdslnamemethod",
		IntroducedIn: "12.0",
		RemovedIn:    "13.0",
		ReplacementPattern: `# Before:
dsl_name :my_resource

# After:
resource_name :my_resource
provides :my_resource`,
	},
	{
		// Lab-verified 2026-07-16: node['key'] = value (no precedence level) raises
		// "Node attributes are read-only when you do not specify which precedence
		// level to use" on CC19.3.15. Chef has required a precedence level since
		// Chef Infra Client 11.
		CopName:      "Chef/Deprecations/NodeSetWithoutLevel",
		Description:  "Setting a node attribute without a precedence level (node['key'] = value) raises at converge time on Chef Infra Client 11 and later. Specify node.default / node.normal / node.override.",
		MigrationURL: "https://docs.chef.io/workstation/cookstyle/chef_deprecations_nodesetwithoutlevel",
		IntroducedIn: "11.0",
		RemovedIn:    "11.0",
		ReplacementPattern: `# Before:
node['my_app']['port'] = 8080

# After:
node.default['my_app']['port'] = 8080`,
	},
	{
		// Lab-verified 2026-07-16: the Chef::PartialSearch constant is undefined on
		// CC19.3.15 (NameError). No removal version in the cop description; RemovedIn
		// is the target on which absence was confirmed.
		CopName:      "Chef/Deprecations/PartialSearchClassUsage",
		Description:  "The legacy Chef::PartialSearch class is undefined on Chef Infra Client 19; direct use raises NameError at converge time. Use the built-in search helper with the :filter_result option.",
		MigrationURL: "https://docs.chef.io/workstation/cookstyle/chef_deprecations_partialsearchclassusage",
		IntroducedIn: "12.0",
		RemovedIn:    "19.0",
		ReplacementPattern: `# Before:
Chef::PartialSearch.new.search(:node, 'role:web', keys: { 'name' => ['name'] })

# After:
search(:node, 'role:web', filter_result: { 'name' => ['name'] })`,
	},
	{
		// Lab-verified 2026-07-16: partial_search is not defined on Chef::DSL::Recipe
		// (nor Universal) on CC19.3.15; a call raises NoMethodError at converge time.
		// No removal version in the cop description; RemovedIn is the confirmed target.
		CopName:      "Chef/Deprecations/PartialSearchHelperUsage",
		Description:  "The legacy partial_search helper is not available on Chef Infra Client 19; a call raises NoMethodError at converge time. Use the built-in search helper with the :filter_result option.",
		MigrationURL: "https://docs.chef.io/workstation/cookstyle/chef_deprecations_partialsearchhelperusage",
		IntroducedIn: "12.0",
		RemovedIn:    "19.0",
		ReplacementPattern: `# Before:
partial_search(:node, 'role:web', keys: { 'name' => ['name'] })

# After:
search(:node, 'role:web', filter_result: { 'name' => ['name'] })`,
	},
	{
		// Lab-verified 2026-07-16: epic_fail is not defined on Chef::DSL::Recipe on
		// CC19.3.15; a call raises NoMethodError at converge time. No removal version
		// in the cop description; RemovedIn is the confirmed target.
		CopName:      "Chef/Deprecations/EpicFail",
		Description:  "The epic_fail helper is not available on Chef Infra Client 19; a call raises NoMethodError at converge time. Use the ignore_failure property on the resource instead.",
		MigrationURL: "https://docs.chef.io/workstation/cookstyle/chef_deprecations_epicfail",
		IntroducedIn: "12.0",
		RemovedIn:    "19.0",
		ReplacementPattern: `# Before:
execute 'thing' do
  epic_fail true
end

# After:
execute 'thing' do
  ignore_failure true
end`,
	},
	{
		CopName:      "Chef/Deprecations/ResourceWithoutUnifiedTrue",
		Description:  "Custom resources should enable unified mode for compatibility with Chef 18+. In unified mode, the resource's compile and converge phases run in a single pass.",
		MigrationURL: "https://docs.chef.io/unified_mode/",
		IntroducedIn: "15.3",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
resource_name :my_resource

# After:
resource_name :my_resource
unified_mode true`,
	},
	{
		CopName:      "Chef/Deprecations/ChefHandlerUsesSupports",
		Description:  "The `supports` method in Chef handlers was deprecated. Use the `type` property instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.0",
		RemovedIn:    "14.0",
		ReplacementPattern: `# Before:
supports :report => true, :exception => true

# After:
type :report`,
	},
	{
		CopName:      "Chef/Deprecations/ChefRewind",
		Description:  "The chef-rewind gem is no longer needed. Use the built-in edit_resource, delete_resource, or find_resource methods instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.10",
		RemovedIn:    "14.0",
		ReplacementPattern: `# Before:
chef_gem 'chef-rewind'
require 'chef/rewind'
rewind 'package[nginx]' do
  version '1.2.3'
end

# After:
edit_resource(:package, 'nginx') do
  version '1.2.3'
end`,
	},
	{
		CopName:      "Chef/Deprecations/ChefSpecifyDefaultAction",
		Description:  "Custom resources should specify a default_action instead of relying on the implicit first action.",
		MigrationURL: "https://docs.chef.io/custom_resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
actions :create, :delete

# After:
default_action :create`,
	},
	{
		CopName:      "Chef/Deprecations/ChefSugarHelpers",
		Description:  "Chef Sugar helpers were merged into Chef Infra Client 15.5+. Remove the chef-sugar gem dependency and use the built-in helpers directly.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "15.5",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
depends 'chef-sugar'
include Chef::Sugar::DataBag

# After:
# Remove the dependency — helpers are built into Chef 15.5+`,
	},
	{
		CopName:      "Chef/Deprecations/CookbookDependsOnCompatResource",
		Description:  "The compat_resource cookbook backported custom resource functionality to older Chef versions. It is no longer needed with Chef 12.19+.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.19",
		RemovedIn:    "14.0",
		ReplacementPattern: `# Before (metadata.rb):
depends 'compat_resource'

# After:
# Remove the dependency`,
	},
	{
		CopName:      "Chef/Deprecations/CookbookDependsOnPartialSearch",
		Description:  "The partial_search cookbook is no longer needed. Partial search is built into Chef Client 12.0+.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.0",
		RemovedIn:    "14.0",
		ReplacementPattern: `# Before (metadata.rb):
depends 'partial_search'

# After:
# Remove the dependency — use search(:node, 'query', filter_result: { ... })`,
	},
	{
		CopName:      "Chef/Deprecations/CookbookDependsOnPoise",
		Description:  "The poise and poise-service cookbooks are no longer maintained and have compatibility issues with modern Chef versions. Migrate to custom resources.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (metadata.rb):
depends 'poise'
depends 'poise-service'

# After:
# Rewrite using custom resources with unified_mode true`,
	},
	{
		CopName:      "Chef/Deprecations/DeprecatedChefSpecHelpers",
		Description:  "Several ChefSpec helper methods have been deprecated. Use the updated method names.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
ChefSpec::Runner.new
stub_command('...')

# After:
ChefSpec::SoloRunner.new
stub_command('...')`,
	},
	{
		// Lab-verified 2026-07-16 on CC19.3.15: the four methods this cop flags —
		// Chef::Platform.provider_for_resource / find_provider /
		// find_provider_for_node / set — are all absent on Chef::Platform (calls
		// raise NoMethodError). Confirmed Blocker (the prior sweep left it :na).
		CopName:      "Chef/Deprecations/DeprecatedPlatformMethods",
		Description:  "The legacy Chef::Platform provider-resolution methods (provider_for_resource, find_provider, find_provider_for_node, set) were removed in Chef Infra Client 15. Calls raise NoMethodError at converge time. Use the resource/provider DSL (provides) instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "13.0",
		RemovedIn:    "15.0",
		ReplacementPattern: `# Before:
Chef::Platform.set(platform: :ubuntu, resource: :package, provider: Chef::Provider::Package::Apt)

# After:
# Use provides in the provider/resource:
provides :package, platform: 'ubuntu'`,
	},
	{
		CopName:      "Chef/Deprecations/DeprecatedShelloutMethods",
		Description:  "The shell_out_with_systems_locale and shell_out_compact methods have been deprecated. Use shell_out with explicit environment options.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.3",
		RemovedIn:    "15.0",
		ReplacementPattern: `# Before:
shell_out_with_systems_locale('command')
shell_out_compact('command', 'arg')

# After:
shell_out('command')
shell_out('command', 'arg')`,
	},
	{
		CopName:      "Chef/Deprecations/DeprecatedWindowsVersionCheck",
		Description:  "Checking Windows version using node['platform_version'] comparison strings is deprecated. Use the new node['platform_version'] Gem::Version comparison or the windows_version helper.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
if node['platform_version'].to_f >= 6.3

# After:
if Gem::Version.new(node['platform_version']) >= Gem::Version.new('6.3')`,
	},
	{
		CopName:      "Chef/Deprecations/DeprecatedYumRepositoryActions",
		Description:  "The :add and :remove actions for yum_repository were replaced by :create and :delete.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.14",
		// No RemovedIn: lab-verified 2026-07-16 that yum_repository still allows
		// action :add on CC19.3.14 (in allowed_actions; why-run converge ran) —
		// deprecated, not removed.
		RemovedIn: "",
		ReplacementPattern: `# Before:
yum_repository 'epel' do
  action :add
end

# After:
yum_repository 'epel' do
  action :create
end`,
	},
	{
		CopName:      "Chef/Deprecations/EasyInstallResource",
		Description:  "The easy_install resource was removed. Use pip or other package management instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.10",
		RemovedIn:    "13.0",
		ReplacementPattern: `# Before:
easy_install_package 'pip-package'

# After:
# Use pip_install or python_package resource from the poise-python cookbook,
# or shell_out to pip directly.`,
	},
	{
		CopName:      "Chef/Deprecations/EOLAuditModeUsage",
		Description:  "Audit mode has been removed from Chef Infra Client. Migrate to Chef InSpec for compliance testing.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "13.0",
		RemovedIn:    "15.0",
		ReplacementPattern: `# Before (client.rb):
audit_mode :audit_only

# After:
# Remove audit_mode — use Chef InSpec with the audit cookbook or
# compliance phase instead.`,
	},
	{
		CopName:      "Chef/Deprecations/ErlCallResource",
		Description:  "The erl_call resource was removed in Chef 13. Use shell_out to call Erlang if needed.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.0",
		RemovedIn:    "13.0",
		ReplacementPattern: `# Before:
erl_call 'my_call' do
  code 'ok.'
end

# After:
execute 'erl_call' do
  command 'erl -eval "ok." -s init stop -noshell'
end`,
	},
	{
		CopName:      "Chef/Deprecations/HWRPWithoutUnifiedTrue",
		Description:  "HWRPs (Heavy Weight Resource Providers) should enable unified mode for compatibility with Chef 18+.",
		MigrationURL: "https://docs.chef.io/unified_mode/",
		IntroducedIn: "15.3",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
class Chef::Provider::MyProvider < Chef::Provider::LWRPBase
  ...
end

# After: Convert to a custom resource with unified_mode true
unified_mode true
provides :my_resource`,
	},
	{
		CopName:      "Chef/Deprecations/LegacyNotifySyntax",
		Description:  "The Hash-based notification syntax has been deprecated. Use the string-based syntax instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "10.0",
		RemovedIn:    "13.0",
		ReplacementPattern: `# Before:
notifies :restart, resources(:service => 'nginx')

# After:
notifies :restart, 'service[nginx]'`,
	},
	{
		CopName:      "Chef/Deprecations/LogResourceNotifications",
		Description:  "Using the log resource to trigger notifications is deprecated. Use the notify_group resource instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "15.8",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
log 'trigger_restart' do
  notifies :restart, 'service[nginx]'
end

# After:
notify_group 'trigger_restart' do
  notifies :restart, 'service[nginx]'
end`,
	},
	{
		CopName:      "Chef/Deprecations/NamePropertyWithDefaultValue",
		Description:  "A property with name_property: true should not also define a default value. The name_property uses the resource name as its default.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "13.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
property :package_name, String, name_property: true, default: 'foo'

# After:
property :package_name, String, name_property: true`,
	},
	{
		CopName:      "Chef/Deprecations/NodeDeepFetch",
		Description:  "node.deep_fetch has been deprecated. Use the dig method or standard hash access instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "15.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
node.deep_fetch('network', 'interfaces', 'eth0', 'addresses')

# After:
node.dig('network', 'interfaces', 'eth0', 'addresses')`,
	},
	{
		CopName:      "Chef/Deprecations/NodeMethodsInsteadofAttributes",
		Description:  "Accessing node attributes using method-style access (node.foo) is deprecated. Use bracket notation (node['foo']) instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "13.0",
		RemovedIn:    "16.0",
		ReplacementPattern: `# Before:
node.platform
node.hostname

# After:
node['platform']
node['hostname']`,
	},
	{
		CopName:      "Chef/Deprecations/NodeSet",
		Description:  "node.set has been deprecated. Use node.normal instead for normal-precedence attributes.",
		MigrationURL: "https://docs.chef.io/deprecations_attributes/",
		IntroducedIn: "12.12",
		RemovedIn:    "14.0",
		ReplacementPattern: `# Before:
node.set['my_cookbook']['port'] = 8080

# After:
node.normal['my_cookbook']['port'] = 8080`,
	},
	{
		CopName:      "Chef/Deprecations/NodeSetUnless",
		Description:  "node.set_unless has been deprecated. Use node.normal_unless instead.",
		MigrationURL: "https://docs.chef.io/deprecations_attributes/",
		IntroducedIn: "12.12",
		RemovedIn:    "14.0",
		ReplacementPattern: `# Before:
node.set_unless['my_cookbook']['port'] = 8080

# After:
node.normal_unless['my_cookbook']['port'] = 8080`,
	},
	{
		CopName:      "Chef/Deprecations/PoiseArchiveUsage",
		Description:  "The poise_archive resource from the poise-archive cookbook is deprecated. Use the archive_file resource built into Chef 15+.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "15.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
poise_archive '/tmp/app.tar.gz' do
  destination '/opt/app'
end

# After:
archive_file '/tmp/app.tar.gz' do
  destination '/opt/app'
end`,
	},
	{
		CopName:      "Chef/Deprecations/ResourceInheritsFromCompatResource",
		Description:  "Custom resources should not inherit from ChefCompat::Resource. This compatibility shim is no longer needed in Chef 12.19+.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.19",
		RemovedIn:    "14.0",
		ReplacementPattern: `# Before:
class MyCookbook::Resource::MyResource < ChefCompat::Resource
  ...
end

# After:
# Convert to a modern custom resource DSL file in resources/`,
	},
	{
		CopName:      "Chef/Deprecations/ResourceOverridesProvidesMethod",
		Description:  "Custom resources should not override the provides? class method. Use the provides DSL method instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "13.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
def self.provides?(node, resource)
  node['platform'] == 'ubuntu'
end

# After:
provides :my_resource, platform: 'ubuntu'`,
	},
	{
		CopName:      "Chef/Deprecations/ResourceUsesOnlyIfNotIf",
		Description:  "Using only_if/not_if with a block that references @new_resource is deprecated. Use properties directly.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
only_if { @new_resource.install_flag }

# After:
only_if { new_resource.install_flag }`,
	},
	{
		CopName:      "Chef/Deprecations/ResourceUsesProviderBaseMethod",
		Description:  "Using Chef::Provider::LWRPBase as a base class for providers is deprecated. Convert to a custom resource.",
		MigrationURL: "https://docs.chef.io/custom_resources/",
		IntroducedIn: "12.0",
		// No RemovedIn: lab-verified 2026-07-16 that Chef::Provider::LWRPBase is
		// still defined and subclassable on CC19.3.14 — deprecated, not removed.
		RemovedIn: "",
		ReplacementPattern: `# Before:
class Chef::Provider::MyProvider < Chef::Provider::LWRPBase
  provides :my_resource
  ...
end

# After: Convert to a custom resource file
# resources/my_resource.rb
unified_mode true
provides :my_resource`,
	},
	{
		CopName:      "Chef/Deprecations/ResourceUsesUpdatedMethod",
		Description:  "Using the deprecated updated=() method in custom resources. Use updated_by_last_action() instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.0",
		RemovedIn:    "13.0",
		ReplacementPattern: `# Before:
@updated = true

# After:
# In a custom resource, converge_if_changed handles this automatically.`,
	},
	{
		CopName:      "Chef/Deprecations/RubyBlockCreateAction",
		Description:  "The :create action for ruby_block is deprecated. Use :run instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.0",
		// No RemovedIn: lab-verified 2026-07-16 that ruby_block still accepts
		// action :create on CC19.3.14 (converges without raising) — deprecated, not
		// removed, so Review not Blocker.
		RemovedIn: "",
		ReplacementPattern: `# Before:
ruby_block 'my_block' do
  action :create
  block do
    # ...
  end
end

# After:
ruby_block 'my_block' do
  action :run
  block do
    # ...
  end
end`,
	},
	{
		CopName:      "Chef/Deprecations/SearchUsesPositionalParameters",
		Description:  "Positional parameters in search() calls are deprecated. Use named parameters instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.0",
		// No RemovedIn: lab-verified 2026-07-16 that Chef::Search::Query#search
		// still accepts positional args on CC19.3.14 — deprecated, not removed.
		RemovedIn: "",
		ReplacementPattern: `# Before:
search(:node, 'role:web', 'name', 0, 1000)

# After:
search(:node, 'role:web', filter_result: { 'name' => ['name'] })`,
	},
	{
		CopName:      "Chef/Deprecations/UseInlineResourcesDefined",
		Description:  "use_inline_resources is now the default in Chef 13+ and should be removed from LWRP/HWRP providers.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.0",
		// No RemovedIn: lab-verified 2026-07-16 that use_inline_resources is still
		// callable on CC19.3.14 (no-op default) — a cleanup item, not a blocker.
		RemovedIn: "",
		ReplacementPattern: `# Before:
use_inline_resources
def whyrun_supported?
  true
end

# After:
# Remove use_inline_resources — it is the default in Chef 13+.
# Convert to a custom resource.`,
	},
	{
		CopName:      "Chef/Deprecations/UsesRunCommandHelper",
		Description:  "The run_command helper is deprecated. Use shell_out or shell_out! instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.0",
		RemovedIn:    "13.0",
		ReplacementPattern: `# Before:
run_command(:command => 'apt-get update')

# After:
shell_out!('apt-get update')`,
	},
	{
		CopName:      "Chef/Deprecations/WindowsFeatureServermanagercmd",
		Description:  "The :servermanagercmd install method for windows_feature is deprecated. Use :dism or :powershell_out instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "15.0",
		ReplacementPattern: `# Before:
windows_feature 'IIS' do
  install_method :servermanagercmd
end

# After:
windows_feature 'IIS' do
  install_method :dism
end`,
	},
	{
		CopName:      "Chef/Deprecations/WindowsPackageInstallerType",
		Description:  "The :installer_type property for windows_package is deprecated in favour of the :source property's auto-detection.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "13.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
windows_package 'MyApp' do
  installer_type :msi
  source 'C:/tmp/myapp.msi'
end

# After:
windows_package 'MyApp' do
  source 'C:/tmp/myapp.msi'
end`,
	},
	{
		CopName:      "Chef/Deprecations/WindowsTaskChangeAction",
		Description:  "The :change action for windows_task is deprecated. Use :create which now supports updating existing tasks.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "13.0",
		// No RemovedIn: lab-verified 2026-07-16 that windows_task still accepts
		// action :change on CC19.3.15 (in allowed_actions, accepted at build) —
		// deprecated, not removed.
		RemovedIn: "",
		ReplacementPattern: `# Before:
windows_task 'my_task' do
  action :change
  ...
end

# After:
windows_task 'my_task' do
  action :create
  ...
end`,
	},
	{
		CopName:      "Chef/Deprecations/DefaultMetadataMaintainer",
		Description:  "The default metadata.rb maintainer and maintainer_email values should be updated from the template defaults.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (metadata.rb):
maintainer 'YOUR_COMPANY_NAME'
maintainer_email 'YOUR_EMAIL'

# After:
maintainer 'My Company'
maintainer_email 'team@example.com'`,
	},
	{
		CopName:      "Chef/Deprecations/PolicyfileCommunitySource",
		Description:  "The :community source in Policyfiles is deprecated. Use the default_source :supermarket instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (Policyfile.rb):
default_source :community

# After:
default_source :supermarket`,
	},
	{
		CopName:      "Chef/Deprecations/RequireRecipe",
		Description:  "require_recipe is deprecated. Use include_recipe instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "10.0",
		RemovedIn:    "14.0",
		ReplacementPattern: `# Before:
require_recipe 'my_cookbook::setup'

# After:
include_recipe 'my_cookbook::setup'`,
	},
	{
		CopName:      "Chef/Deprecations/ChefWindowsPlatformHelper",
		Description:  "Chef::Platform.windows? is deprecated. Use the platform?('windows') helper or node['os'] == 'windows'.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
if Chef::Platform.windows?

# After:
if platform?('windows')`,
	},
	{
		CopName:      "Chef/Deprecations/VerifyPropertyUsesFileExpansion",
		Description:  "Using file expansion (%{path}) in verify properties is deprecated. Use the block form of verify instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.5",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
file '/etc/config' do
  verify 'configcheck %{path}'
end

# After:
file '/etc/config' do
  verify do |path|
    shell_out("configcheck #{path}").exitstatus == 0
  end
end`,
	},
	{
		CopName:      "Chef/Deprecations/UseAutomaticResourceName",
		Description:  "use_automatic_resource_name is deprecated. Use provides with the resource name explicitly.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "16.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
use_automatic_resource_name

# After:
provides :my_resource_name`,
	},
	{
		CopName:      "Chef/Deprecations/DependsOnOmnibusUpdaterCookbook",
		Description:  "The omnibus_updater cookbook is deprecated. Use the chef_client_updater cookbook instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (metadata.rb):
depends 'omnibus_updater'

# After:
depends 'chef_client_updater'`,
	},
	{
		CopName:      "Chef/Deprecations/DependsOnChefNginxCookbook",
		Description:  "The chef_nginx cookbook (formerly nginx) is deprecated. Use the sous-chefs nginx cookbook or write custom resources.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (metadata.rb):
depends 'chef_nginx'

# After:
# Evaluate alternatives or write custom nginx resources.`,
	},
	{
		CopName:      "Chef/Deprecations/FoodcriticTesting",
		Description:  "Foodcritic has been replaced by CookStyle (Cookstyle). Remove Foodcritic configuration and use CookStyle instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (Rakefile / Gemfile):
require 'foodcritic'
gem 'foodcritic'

# After:
require 'cookstyle'
gem 'cookstyle'`,
	},
	{
		CopName:      "Chef/Deprecations/LibrarianChefSpec",
		Description:  "Librarian-Chef is deprecated. Use Berkshelf or Policyfiles for dependency resolution.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
# Cheffile for librarian-chef

# After:
# Use Berksfile or Policyfile.rb`,
	},
	{
		CopName:      "Chef/Deprecations/UserDeprecatedSupportsProperty",
		Description:  "The user resource's supports property (for manage_home, non_unique) was removed in Chef Infra Client 13. Use the individual properties directly.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "12.14",
		RemovedIn:    "13.0",
		ReplacementPattern: `# Before:
user 'deploy' do
  supports manage_home: true
end

# After:
user 'deploy' do
  manage_home true
end`,
	},
	{
		CopName:      "Chef/Deprecations/MacosUserdefaultsGlobalProperty",
		Description:  "The macos_userdefaults resource's global property is deprecated. Omit the domain property to write to the global domain.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "15.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
macos_userdefaults 'set dock size' do
  global true
  key 'AppleDockSize'
  value 36
end

# After:
macos_userdefaults 'set dock size' do
  key 'AppleDockSize'
  value 36
end`,
	},
	{
		CopName:      "Chef/Deprecations/IncludingYumDNFCompatRecipe",
		Description:  "Including the yum::dnf_yum_compat recipe is no longer needed. The yum_package resource handles dnf natively in modern Chef.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "15.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
include_recipe 'yum::dnf_yum_compat'

# After:
# Remove the include_recipe line.`,
	},
	{
		CopName:      "Chef/Deprecations/IncludingXMLRubyCookbook",
		Description:  "The xml::ruby recipe is no longer needed. The nokogiri gem is included in Chef Infra Client.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "15.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
include_recipe 'xml::ruby'

# After:
# Remove the include_recipe line.`,
	},
	{
		CopName:      "Chef/Deprecations/DependsOnChefHandlerCookbook",
		Description:  "The chef_handler cookbook is no longer needed. The chef_handler resource is built into Chef 14+.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "14.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (metadata.rb):
depends 'chef_handler'

# After:
# Remove the dependency — use the built-in chef_handler resource.`,
	},
	{
		CopName:      "Chef/Deprecations/RubyVersionConstraintInEnvironment",
		Description:  "Specifying a Ruby version constraint in a Chef environment is deprecated.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "13.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (environment.rb):
ruby_version '~> 2.5'

# After:
# Remove the ruby_version constraint.`,
	},
	{
		CopName:      "Chef/Deprecations/ChefDKGenerators",
		Description:  "ChefDK generators are deprecated. Use Chef Workstation's chef generate command instead.",
		MigrationURL: "https://docs.chef.io/deprecations/",
		IntroducedIn: "15.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
chef generate cookbook -g ~/chefdk_generators my_cookbook

# After:
chef generate cookbook my_cookbook`,
	},

	// -----------------------------------------------------------------------
	// Chef/Correctness
	// -----------------------------------------------------------------------
	{
		CopName:      "Chef/Correctness/BlockGuardWithOnlyString",
		Description:  "A guard (only_if/not_if) with a block should not contain a single string. Use the string form of the guard instead.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
only_if { 'test -f /etc/config' }

# After:
only_if 'test -f /etc/config'`,
	},
	{
		CopName:      "Chef/Correctness/ConditionalRubyShellout",
		Description:  "A guard should not use shell_out inside a Ruby block when a string guard would suffice.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
only_if { shell_out('test -f /etc/config').exitstatus == 0 }

# After:
only_if 'test -f /etc/config'`,
	},
	{
		CopName:      "Chef/Correctness/CookbookUsesNodeSave",
		Description:  "Calling node.save in a recipe can cause unexpected data overwrites and performance issues. Remove node.save calls.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
node.save

# After:
# Remove node.save — the node is saved automatically at the end of a run.`,
	},
	{
		CopName:      "Chef/Correctness/DnfPackageAllowDowngrades",
		Description:  "The dnf_package resource does not support the allow_downgrades property. Use a specific version constraint instead.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "15.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
dnf_package 'nginx' do
  allow_downgrades true
  version '1.18'
end

# After:
dnf_package 'nginx' do
  version '1.18'
end`,
	},
	{
		CopName:      "Chef/Correctness/IncorrectLibraryInjection",
		Description:  "Library helpers should be injected using ::Chef::DSL::Recipe.include or Chef::Resource.include, not by reopening core classes.",
		MigrationURL: "https://docs.chef.io/libraries/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
class Chef::Recipe
  def my_helper
    ...
  end
end

# After:
module MyCookbook
  module Helpers
    def my_helper
      ...
    end
  end
end
Chef::DSL::Recipe.include MyCookbook::Helpers`,
	},
	{
		CopName:      "Chef/Correctness/InvalidPlatformMetadata",
		Description:  "The supports metadata in metadata.rb contains invalid platform names. Check the platform name against the list of known Ohai platforms.",
		MigrationURL: "https://docs.chef.io/config_rb_metadata/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (metadata.rb):
supports 'ubuntu'
supports 'centOS'  # wrong case

# After:
supports 'ubuntu'
supports 'centos'`,
	},
	{
		CopName:      "Chef/Correctness/InvalidVersionMetadata",
		Description:  "The version in metadata.rb is not a valid semantic version.",
		MigrationURL: "https://docs.chef.io/config_rb_metadata/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (metadata.rb):
version '1.0'

# After:
version '1.0.0'`,
	},
	{
		CopName:      "Chef/Correctness/MetadataMissingName",
		Description:  "The name field is missing from metadata.rb. This is required by Chef 12+.",
		MigrationURL: "https://docs.chef.io/config_rb_metadata/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before (metadata.rb):
maintainer 'My Company'
version '1.0.0'

# After:
name 'my_cookbook'
maintainer 'My Company'
version '1.0.0'`,
	},
	{
		CopName:      "Chef/Correctness/NodeNormal",
		Description:  "Setting node.normal attributes persists data to the Chef server after every run, which can cause unexpected behavior. Use node.default or node.override instead.",
		MigrationURL: "https://docs.chef.io/attributes/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
node.normal['my_app']['config'] = 'value'

# After:
node.default['my_app']['config'] = 'value'`,
	},
	{
		CopName:      "Chef/Correctness/NodeNormalUnless",
		Description:  "Setting node.normal_unless attributes persists data to the Chef server. Use node.default_unless or node.override_unless instead.",
		MigrationURL: "https://docs.chef.io/attributes/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
node.normal_unless['my_app']['port'] = 8080

# After:
node.default_unless['my_app']['port'] = 8080`,
	},
	{
		CopName:      "Chef/Correctness/NotifiesActionNotSymbol",
		Description:  "The action in a notifies statement must be a symbol, not a string.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
notifies 'restart', 'service[nginx]'

# After:
notifies :restart, 'service[nginx]'`,
	},
	{
		CopName:      "Chef/Correctness/ResourceSetsInternalProperties",
		Description:  "Setting internal properties (such as :updated, :executed) on a resource is incorrect and can cause unexpected behavior.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
service 'nginx' do
  updated true
end

# After:
# Remove the internal property assignment.`,
	},
	{
		CopName:      "Chef/Correctness/ResourceSetsNameProperty",
		Description:  "A resource should not set the name property in the resource body since it is automatically set from the resource declaration.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
service 'nginx' do
  service_name 'nginx'
end

# After:
service 'nginx'`,
	},
	{
		CopName:      "Chef/Correctness/ResourceWithNoneAction",
		Description:  "Resources should not use :none as an action. Use :nothing instead.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
service 'nginx' do
  action :none
end

# After:
service 'nginx' do
  action :nothing
end`,
	},
	{
		CopName:      "Chef/Correctness/ScopedFileExist",
		Description:  "Use ::File.exist? instead of File.exist? in recipes to avoid namespace conflicts with the Chef file resource.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
if File.exist?('/etc/config')

# After:
if ::File.exist?('/etc/config')`,
	},
	{
		CopName:      "Chef/Correctness/ServiceResource",
		Description:  "The service resource should not be used with both start and enable actions in separate resource blocks for the same service. Combine them.",
		MigrationURL: "https://docs.chef.io/resources/service/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
service 'nginx' do
  action :start
end
service 'nginx' do
  action :enable
end

# After:
service 'nginx' do
  action [:enable, :start]
end`,
	},
	{
		CopName:      "Chef/Correctness/TmpPath",
		Description:  "Using /tmp in resource paths is not recommended. Use Chef::Config[:file_cache_path] or a platform-appropriate temporary directory.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
remote_file '/tmp/installer.sh' do
  source 'https://example.com/installer.sh'
end

# After:
remote_file "#{Chef::Config[:file_cache_path]}/installer.sh" do
  source 'https://example.com/installer.sh'
end`,
	},
	{
		CopName:      "Chef/Correctness/InvalidNotificationTiming",
		Description:  "Notification timing must be :delayed (default), :immediately, or :before. Other values are invalid.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
notifies :restart, 'service[nginx]', :right_away

# After:
notifies :restart, 'service[nginx]', :immediately`,
	},
	{
		CopName:      "Chef/Correctness/InvalidDefaultAction",
		Description:  "The default_action in a custom resource must be a symbol or array of symbols, not a string.",
		MigrationURL: "https://docs.chef.io/custom_resources/",
		IntroducedIn: "12.5",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
default_action 'create'

# After:
default_action :create`,
	},
	{
		CopName:      "Chef/Correctness/LazyEvalNodeAttributeDefaults",
		Description:  "Node attributes used in property defaults must be wrapped in lazy {} to avoid compile-time evaluation.",
		MigrationURL: "https://docs.chef.io/custom_resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
property :port, Integer, default: node['my_app']['port']

# After:
property :port, Integer, default: lazy { node['my_app']['port'] }`,
	},
	{
		CopName:      "Chef/Correctness/ChefApplicationFatal",
		Description:  "Calling Chef::Application.fatal! in a recipe or resource halts the entire Chef run. Use raise or Chef::Log.fatal with a resource guard instead.",
		MigrationURL: "https://docs.chef.io/resources/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
Chef::Application.fatal!('Configuration missing!', 1)

# After:
raise 'Configuration missing!'`,
	},
	{
		CopName:      "Chef/Correctness/PowershellScriptDeleteFile",
		Description:  "Using powershell_script to delete a file is unnecessary. Use the file resource with action :delete.",
		MigrationURL: "https://docs.chef.io/resources/file/",
		IntroducedIn: "12.0",
		RemovedIn:    "",
		ReplacementPattern: `# Before:
powershell_script 'delete file' do
  code 'Remove-Item C:\temp\file.txt'
end

# After:
file 'C:\temp\file.txt' do
  action :delete
end`,
	},
}
