// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

// Schemas, read off the Go types the service actually decodes and encodes.
//
// The same rule as the rest of the description: derived from what is served,
// never written beside it. A hand-kept table of fields is the failure that
// killed the document set the journeys replaced, and it fails worst here —
// somebody reads it, believes it, and their unattended job breaks at three in
// the morning with nobody watching.
//
// So the only thing declared anywhere is *which type* an address decodes. The
// fields, their names on the wire and what they hold are reflected out of that
// type, which means adding a field to a handler's body changes the description
// in the same commit, and renaming one cannot leave a stale name behind.
//
// What is deliberately NOT derived is requiredness. The handlers enforce it by
// hand — `if body.Name == "" { WriteBadRequest }` — and reflection cannot see
// that. Deriving it from, say, the absence of a pointer would be a guess, and a
// guess in a generated client is a constraint a caller cannot get past even
// when the service would have accepted the call. Silence costs one clear 400.

const schemaRefPrefix = "#/components/schemas/"

// schemaRegistry turns Go types into JSON Schema, defining each named type once
// under components/schemas and referring to it everywhere else. One model per
// type in a generated client rather than a fresh anonymous copy per operation.
type schemaRegistry struct {
	defs map[string]any
}

func newSchemaRegistry() *schemaRegistry {
	return &schemaRegistry{defs: map[string]any{}}
}

// components is what goes under components/schemas in the document.
func (reg *schemaRegistry) components() map[string]any {
	return reg.defs
}

// schemaComponentName is the name a type is defined under. Package-qualified
// because two packages here do have same-named types, and a silent collision
// would describe one of them as the other.
func schemaComponentName(t reflect.Type) string {
	pkg := t.PkgPath()
	if i := strings.LastIndex(pkg, "/"); i >= 0 {
		pkg = pkg[i+1:]
	}
	if pkg == "" {
		return t.Name()
	}
	return pkg + "." + t.Name()
}

// goTypeName writes a type the way it is written in the source, so a
// declaration made in Go and a declaration read out of the source can be
// compared. "[]config.Organisation", "webapi.loginRequest", "[]string".
//
// Used only by the tests that hold the declared bodies and the decoded ones in
// step; nothing in the served description depends on it.
func goTypeName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Pointer:
		return goTypeName(t.Elem())
	case reflect.Slice, reflect.Array:
		return "[]" + goTypeName(t.Elem())
	}
	if t.PkgPath() == "" {
		return t.Name() // a builtin: string, int
	}
	return schemaComponentName(t)
}

// schemaFor describes a type: a reference for a named struct, the shape itself
// for anything else.
func (reg *schemaRegistry) schemaFor(t reflect.Type) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	// Two types decide their own shape and would be described wrongly by
	// walking their fields.
	switch t {
	case reflect.TypeOf(time.Time{}):
		return map[string]any{"type": "string", "format": "date-time"}
	case reflect.TypeOf(json.RawMessage{}):
		// Genuinely anything. The service does not decide this shape — see the
		// node-ingest note in plans — so pinning it to an object would refuse a
		// caller something the handler would have taken.
		return map[string]any{}
	}

	if t.Kind() != reflect.Struct || t.Name() == "" {
		return reg.inlineSchema(t)
	}

	// Registered before the fields are walked, so a type that contains itself
	// finds a reference waiting rather than recursing forever.
	name := schemaComponentName(t)
	if _, seen := reg.defs[name]; !seen {
		reg.defs[name] = map[string]any{"type": "object"}
		reg.defs[name] = reg.structSchema(t)
	}
	return map[string]any{"$ref": schemaRefPrefix + name}
}

// pageOf describes the standard paginated envelope carrying a list of elem.
//
// The envelope is read off PaginatedResponse rather than written out here, so
// the metadata a caller is promised beside the rows is the metadata the service
// really sends, down to the field names. Only data is substituted, because the
// envelope carries it as `any` and reflection can learn nothing from that —
// which row type it holds is the one thing the address has to declare.
func (reg *schemaRegistry) pageOf(elem reflect.Type) map[string]any {
	envelope := reg.structSchema(reflect.TypeOf(PaginatedResponse{}))
	props, _ := envelope["properties"].(map[string]any)
	if props == nil {
		return envelope
	}
	props["data"] = map[string]any{"type": "array", "items": reg.schemaFor(elem)}
	return envelope
}

// inlineSchema describes anything that is not a named struct.
func (reg *schemaRegistry) inlineSchema(t reflect.Type) map[string]any {
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice, reflect.Array:
		// A byte slice is not a list of numbers on the wire; the decoder reads
		// it as base64 text.
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "format": "byte"}
		}
		return map[string]any{"type": "array", "items": reg.schemaFor(t.Elem())}
	case reflect.Map:
		return map[string]any{
			"type":                 "object",
			"additionalProperties": reg.schemaFor(t.Elem()),
		}
	case reflect.Struct:
		return reg.structSchema(t)
	case reflect.Interface:
		// `any` accepts whatever arrives, and saying so is the honest answer.
		return map[string]any{}
	default:
		return map[string]any{}
	}
}

// structSchema walks the fields the JSON decoder honours, and only those.
func (reg *schemaRegistry) structSchema(t reflect.Type) map[string]any {
	props := map[string]any{}
	reg.collectFields(t, props)
	return map[string]any{"type": "object", "properties": props}
}

// collectFields reads one struct's fields into props, following embedded
// structs because the decoder flattens them — a caller sends those fields at
// the top level, so that is where the description has to put them.
func (reg *schemaRegistry) collectFields(t reflect.Type, props map[string]any) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name, ok := wireName(field)
		if !ok {
			continue
		}

		if field.Anonymous && name == "" {
			embedded := field.Type
			for embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				reg.collectFields(embedded, props)
				continue
			}
		}
		if name == "" {
			name = field.Name
		}
		props[name] = reg.schemaFor(field.Type)
	}
}

// wireName is the name the decoder honours for a field, and whether the field
// appears on the wire at all. Describing the Go name instead would send a
// caller a field the service silently drops.
//
// The yaml tag is the fallback because the settings sections are read by the
// YAML decoder — which accepts JSON and honours yaml tags — so for those types
// it is the yaml tag that says what a caller may send.
func wireName(field reflect.StructField) (string, bool) {
	tag, tagged := field.Tag.Lookup("json")
	if !tagged {
		tag, tagged = field.Tag.Lookup("yaml")
	}
	if !tagged {
		if field.Anonymous {
			return "", true
		}
		return field.Name, true
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" && !strings.Contains(tag, ",") {
		return "", false
	}
	if name == "" && !field.Anonymous {
		return field.Name, true
	}
	return name, true
}
