//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// The journey suite for journeys/service-configuration.md. Run it with
// `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do, so running this recomputes the todo list rather than
// asking anybody to keep one true. Outside the gating suite on purpose: a red
// here is a gap, never a broken build.
//
// The suite lives in this package because the store, the assembly of a
// configuration out of it, and the live swap are all here. What each component
// then does with a value it is handed is spread across the tree and cannot be
// reached from one place; that line is recorded as a skip rather than left out.

// configJourneyRoundTrip writes a configuration to sections and reads it back,
// which is what the service does between somebody pressing save and a component
// reading the value.
func configJourneyRoundTrip(t *testing.T, in *config.Config) *config.Config {
	t.Helper()
	sections, err := ConfigToSections(in)
	if err != nil {
		t.Fatalf("turning a configuration into what the store holds: %v", err)
	}
	out, err := AssembleConfigRaw(map[string]json.RawMessage(sections))
	if err != nil {
		t.Fatalf("building a configuration back out of the store: %v", err)
	}
	return out
}

// "Every setting reachable from the interface: what we collect and how often,
// which Chef servers, the version we are moving to, how much work runs at once,
// how noisy the logging is, how long things are kept, what the analysis tools
// do, how exports behave, how names are displayed."
//
// The journey's own "what proves it" says nothing holds this, and calls it the
// gap most likely to bite. It can be held, at the level of whole settings: a
// setting the store has no section for cannot be set from the interface no
// matter what the interface offers.
//
// This is the test that fails when somebody adds a configuration field and does
// not wire it — which is the failure the journey describes, arriving as a red
// rather than as a value that silently does nothing.
func TestJourney_EverySettingIsReachableFromTheStore(t *testing.T) {
	sections, err := ConfigToSections(&config.Config{})
	if err != nil {
		t.Fatalf("turning a configuration into what the store holds: %v", err)
	}

	// The two values that unlock the database itself. The journey names them
	// as the only legitimate exceptions: they cannot live in the thing they
	// unlock, so they are reached before the store exists.
	bootstrap := map[string]bool{
		"datastore":                     true,
		"credential_encryption_key_env": true,
	}

	rt := reflect.TypeOf(config.Config{})
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if tag == "" || tag == "-" || bootstrap[tag] {
			continue
		}

		var reachable bool
		for key := range sections {
			if key == tag || strings.HasPrefix(key, tag+".") {
				reachable = true
				break
			}
		}
		if !reachable {
			t.Errorf("%q cannot be set from the store at all, so whatever the interface "+
				"offers for it is a field that does nothing — and on this deployment there "+
				"is no other way to change it", tag)
		}
	}
}

// "Changes to take effect now. Not on the next restart — now, on the next
// cycle, on the next request."
//
// Whatever is written has to come back as what was written. A setting that is
// silently altered on the way through is a setting somebody changes twice and
// then stops believing.
func TestJourney_WhatIChangeIsWhatComesBack(t *testing.T) {
	settled := configJourneyRoundTrip(t, &config.Config{})
	again := configJourneyRoundTrip(t, settled)

	rt := reflect.TypeOf(config.Config{})
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		wrote := reflect.ValueOf(*settled).Field(i).Interface()
		read := reflect.ValueOf(*again).Field(i).Interface()
		if !reflect.DeepEqual(wrote, read) {
			t.Errorf("%s does not survive being saved and read back, so changing it from "+
				"the screen does not leave it changed", field.Name)
		}
	}
}

// "Nothing caches a setting at startup. A component that reads its
// configuration once and holds it is indistinguishable from a component that
// requires a restart."
//
// The mechanism: readers take the current configuration rather than a copy
// taken when they were built.
func TestJourney_ReadersGetTheCurrentConfigurationNotACopy(t *testing.T) {
	holder := NewConfigHolder(&config.Config{TargetChefVersion: "18.0.0"}, nil)

	before := holder.Get()
	holder.Set(&config.Config{TargetChefVersion: "19.0.0"})
	after := holder.Get()

	if before == after {
		t.Fatal("a reader that fetched the configuration before a change gets the same " +
			"object afterwards, so the change never reaches it")
	}
	if after.TargetChefVersion != "19.0.0" {
		t.Errorf("after a change the current configuration still reads %q, so a component "+
			"reading it now is working from the old value", after.TargetChefVersion)
	}
}

// "For a bad setting not to take the service down with it." and "A rejected
// change leaves the running configuration alone. The previous good
// configuration keeps running."
//
// This is the lockout failure arriving by a different route, so it is asserted
// from the person's direction: after a change that cannot be applied, the
// service is still running the configuration it was running before.
func TestJourney_ARejectedChangeLeavesTheRunningServiceAlone(t *testing.T) {
	good := &config.Config{TargetChefVersion: "18.0.0"}
	holder := NewConfigHolder(good, nil)

	// A reload with no store behind it cannot succeed. Whatever the failure,
	// the running configuration must be the one that was already working.
	if err := holder.Reload(context.Background()); err == nil {
		t.Skip("a reload with no store attached succeeded, so this cannot exercise the " +
			"failing case from here")
	}

	if got := holder.Get(); got == nil {
		t.Fatal("after a change that could not be applied there is no running " +
			"configuration at all, which takes the service down over a typo")
	} else if got.TargetChefVersion != "18.0.0" {
		t.Errorf("after a change that could not be applied the service is running %q "+
			"rather than the configuration that was already working, so a bad setting "+
			"replaces a good one", got.TargetChefVersion)
	}
}

// "To be told when I have typed something impossible, at the point I type it,
// rather than finding out because collection stopped overnight."
//
// Refusal at the point of setting, not at the point of firing. Assembling a
// configuration out of the store does not judge it — the judging is a separate
// step, and this asks that the step actually refuses. Whether the interface
// runs it before storing rather than after is decided on the save path in
// internal/webapi and is not reachable from here.
func TestJourney_ImpossibleValuesAreRefused(t *testing.T) {
	cfg, err := AssembleConfigRaw(map[string]json.RawMessage{
		KeyServerTLS: json.RawMessage(`{"mode":"not-a-mode-anybody-implements"}`),
	})
	if err != nil {
		// Refused on the way out of the store, which is earlier still.
		return
	}

	if _, err := cfg.Validate(); err == nil {
		t.Error("a configuration naming an encryption mode nothing implements was accepted, " +
			"so the person who typed it is told nothing and finds out when the service " +
			"next tries to listen")
	}
}

// "Configuration lives in the database and is edited through the interface.
// That is the whole model, not the convenient case. The only legitimate
// exceptions are the two values that unlock the database itself."
//
// Two, and only two. A third exception is how the model erodes: each one
// arrives looking reasonable on its own, and each makes the deployment less
// operable by the person who has to operate it.
func TestJourney_ThereAreOnlyTwoValuesThatComeFromOutside(t *testing.T) {
	sections, err := ConfigToSections(&config.Config{})
	if err != nil {
		t.Fatalf("turning a configuration into what the store holds: %v", err)
	}

	var outside []string
	rt := reflect.TypeOf(config.Config{})
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		var reachable bool
		for key := range sections {
			if key == tag || strings.HasPrefix(key, tag+".") {
				reachable = true
				break
			}
		}
		if !reachable {
			outside = append(outside, tag)
		}
	}

	if len(outside) > 2 {
		t.Errorf("%d settings can only be set from outside the interface (%s); the model "+
			"allows exactly two, the ones that unlock the database itself",
			len(outside), strings.Join(outside, ", "))
	}
}

// "A field being declared as configurable in code is not evidence that it can
// be set — that question is answered by whether it is wired into the store."
//
// Held by TestJourney_EverySettingIsReachableFromTheStore, which asks the
// store rather than the declaration. Kept as its own line because the journey
// states it as a rule for the reader, and a rule with no test beside it is the
// kind that gets re-litigated.
func TestJourney_ADeclarationInCodeIsNotEvidenceASettingCanBeSet(t *testing.T) {
	if len(AllConfigKeys()) == 0 {
		t.Fatal("the store knows of no settings at all, so nothing can be set from the " +
			"interface and every declared field is a field that does nothing")
	}
}

// "Nothing proves a change reaches every component live. The mechanism is
// proven; each component's use of it is not. A component that took a copy at
// construction time would pass all of the above."
func TestJourney_EveryComponentReadsItsSettingLive(t *testing.T) {
	t.Skip("whether each component reads through the live accessor rather than caching a " +
		"value when it was built is a property of every package that holds configuration, " +
		"and cannot be asked from here; the mechanism is held by " +
		"TestJourney_ReadersGetTheCurrentConfigurationNotACopy")
}
