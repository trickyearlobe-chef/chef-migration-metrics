// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// The schema generator reads the type the handler actually decodes into. Every
// test here is about one thing: what it says has to be something the type
// really carries, because a description that guesses is worse than one that is
// silent — a caller believes it.

type schemaScalars struct {
	Name    string  `json:"name"`
	Count   int     `json:"count"`
	Ratio   float64 `json:"ratio"`
	Enabled bool    `json:"enabled"`
}

func TestSchema_ScalarsCarryTheirType(t *testing.T) {
	props := schemaProperties(t, schemaScalars{})

	for field, want := range map[string]string{
		"name":    "string",
		"count":   "integer",
		"ratio":   "number",
		"enabled": "boolean",
	} {
		got, ok := props[field].(map[string]any)
		if !ok {
			t.Errorf("%s is missing from the schema, so a generated client has no such field",
				field)
			continue
		}
		if got["type"] != want {
			t.Errorf("%s is described as %v, but the handler decodes it as %s — a client will "+
				"send the wrong thing and get a 400 it cannot explain", field, got["type"], want)
		}
	}
}

type schemaTagged struct {
	Renamed  string `json:"renamed_field"`
	Skipped  string `json:"-"`
	Untagged string
	unseen   string //nolint:unused // exercises that unexported fields are not described
}

// The json tag is what the decoder honours, so it is what the description has
// to say. Describing the Go field name instead sends a caller a field the
// service will silently ignore.
func TestSchema_UsesTheNameTheDecoderHonours(t *testing.T) {
	props := schemaProperties(t, schemaTagged{})

	if _, ok := props["renamed_field"]; !ok {
		t.Error("a renamed field is described under its Go name, not the name on the wire, so " +
			"anything a caller sends is dropped without a word")
	}
	if _, ok := props["Renamed"]; ok {
		t.Error("the Go field name is described as if it were the wire name")
	}
	if _, ok := props["Skipped"]; ok {
		t.Error("a field the decoder ignores is described as if a caller could set it")
	}
	if _, ok := props["Untagged"]; !ok {
		t.Error("an untagged field is not described, but the decoder still accepts it under its " +
			"Go name — so the description is missing something that works")
	}
	if _, ok := props["unseen"]; ok {
		t.Error("an unexported field is described, but nothing outside can ever set it")
	}
}

type schemaYAMLTagged struct {
	Schedule   string `yaml:"schedule"`
	ThresholdD int    `yaml:"stale_node_threshold_days"`
	Both       string `json:"json_name" yaml:"yaml_name"`
}

// The settings sections are decoded by the YAML reader, which accepts JSON and
// honours yaml tags — so for those types the yaml tag is the name on the wire.
// Reading the json tag alone would describe every settings field under its Go
// name, which is the exact fault this generator exists to prevent, applied to
// sixteen write calls at once.
func TestSchema_FallsBackToTheYAMLTagWhenThatIsWhatTheDecoderReads(t *testing.T) {
	props := schemaProperties(t, schemaYAMLTagged{})

	for _, field := range []string{"schedule", "stale_node_threshold_days"} {
		if _, ok := props[field]; !ok {
			t.Errorf("%q is described under its Go name rather than the name the settings "+
				"decoder honours, so a caller sets a field that is silently ignored", field)
		}
	}
	if _, ok := props["Schedule"]; ok {
		t.Error("a yaml-tagged field is described by its Go name")
	}
	// json wins where both exist: anything carrying both is served as JSON.
	if _, ok := props["json_name"]; !ok {
		t.Error("a field carrying both tags is not described under its JSON name")
	}
	if _, ok := props["yaml_name"]; ok {
		t.Error("a field carrying both tags is described under its YAML name, but it is served " +
			"as JSON")
	}
}

type schemaOptional struct {
	Always  string  `json:"always"`
	Perhaps *string `json:"perhaps"`
}

// A pointer in these handlers means "absent leaves it unchanged" — that is a
// real distinction the type carries, and the only one it carries about whether
// a field has to be sent.
//
// Requiredness is deliberately NOT derived. The handlers enforce it by hand
// ("if body.Name == ”"), which reflection cannot see, so claiming a field is
// required would be a guess. Being silent costs a caller one clear 400; being
// wrong costs them a client generated around a constraint that is not real.
func TestSchema_SaysNothingItCannotKnowAboutRequiredness(t *testing.T) {
	schema := schemaOf(t, schemaOptional{})

	if _, ok := schema["required"]; ok {
		t.Error("the schema claims to know which fields are required, but requiredness is " +
			"checked inside the handler where nothing here can see it — so the claim is a guess")
	}

	props, _ := schema["properties"].(map[string]any)
	perhaps, _ := props["perhaps"].(map[string]any)
	if perhaps == nil {
		t.Fatal("the optional field is missing entirely")
	}
	if perhaps["type"] != "string" {
		t.Errorf("a *string is described as %v rather than the string it decodes", perhaps["type"])
	}
}

type schemaNested struct {
	Items []schemaScalars    `json:"items"`
	Free  map[string]string  `json:"free"`
	Raw   json.RawMessage    `json:"raw"`
	When  time.Time          `json:"when"`
	Deep  []schemaTagged     `json:"deep"`
	Inner struct{ A string } `json:"inner"`
}

func TestSchema_ContainersDescribeWhatTheyHold(t *testing.T) {
	props := schemaProperties(t, schemaNested{})

	items, _ := props["items"].(map[string]any)
	if items["type"] != "array" {
		t.Errorf("a slice is described as %v rather than an array", items["type"])
	}
	if items["items"] == nil {
		t.Error("an array does not say what it holds, so a client generator emits a list of " +
			"nothing and the caller has to read our source to find out")
	}

	free, _ := props["free"].(map[string]any)
	if free["type"] != "object" || free["additionalProperties"] == nil {
		t.Errorf("a map is described as %v with no value type, so a caller cannot tell what it "+
			"may put in it", free["type"])
	}

	when, _ := props["when"].(map[string]any)
	if when["type"] != "string" || when["format"] != "date-time" {
		t.Errorf("a time is described as %v/%v rather than a date-time string, which is what "+
			"the decoder actually accepts", when["type"], when["format"])
	}
}

// Raw JSON is the one place the service genuinely does not decide the shape.
// Describing it as an object would be inventing a constraint; it has to read as
// "anything", because that is what is accepted.
func TestSchema_RawJSONIsDescribedAsAnything(t *testing.T) {
	props := schemaProperties(t, schemaNested{})

	raw, ok := props["raw"].(map[string]any)
	if !ok {
		t.Fatal("a raw JSON field is missing from the schema entirely")
	}
	if raw["type"] != nil {
		t.Errorf("raw JSON is pinned to type %v, but the handler accepts whatever arrives — a "+
			"client generated from this refuses to send something the service would have taken",
			raw["type"])
	}
}

type schemaRecursive struct {
	Name     string             `json:"name"`
	Children []*schemaRecursive `json:"children"`
}

// A type that contains itself must not send the generator round forever.
func TestSchema_HandlesATypeThatContainsItself(t *testing.T) {
	done := make(chan map[string]any, 1)
	go func() {
		reg := newSchemaRegistry()
		done <- reg.schemaFor(reflect.TypeOf(schemaRecursive{}))
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("generating a schema for a self-referencing type does not finish, so the " +
			"description cannot be served at all")
	}
}

// A named type is registered once and referred to, so a client generator emits
// one model rather than a fresh anonymous copy per operation.
func TestSchema_NamedTypesAreSharedRatherThanRepeated(t *testing.T) {
	reg := newSchemaRegistry()
	first := reg.schemaFor(reflect.TypeOf(schemaScalars{}))
	second := reg.schemaFor(reflect.TypeOf(schemaScalars{}))

	ref, ok := first["$ref"].(string)
	if !ok {
		t.Fatalf("a named type is inlined rather than referred to (%v), so every operation that "+
			"takes one gets its own copy in a generated client", first)
	}
	if second["$ref"] != ref {
		t.Errorf("the same type is referred to two different ways (%q then %v)", ref, second["$ref"])
	}
	if _, ok := reg.components()[schemaComponentName(reflect.TypeOf(schemaScalars{}))]; !ok {
		t.Error("a type is referred to but never defined, so the reference dangles and the " +
			"document will not load in standard tooling")
	}
}

// schemaOf resolves a type to its schema body, following the reference if the
// registry made one, so a test can read the properties either way.
func schemaOf(t *testing.T, v any) map[string]any {
	t.Helper()
	reg := newSchemaRegistry()
	schema := reg.schemaFor(reflect.TypeOf(v))
	if ref, ok := schema["$ref"].(string); ok {
		name := ref[len("#/components/schemas/"):]
		resolved, ok := reg.components()[name].(map[string]any)
		if !ok {
			t.Fatalf("the schema refers to %q, which the registry does not define", name)
		}
		return resolved
	}
	return schema
}

func schemaProperties(t *testing.T, v any) map[string]any {
	t.Helper()
	props, ok := schemaOf(t, v)["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%T produced no properties at all", v)
	}
	return props
}
