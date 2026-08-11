// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

// What each address is for, in one line, keyed by "METHOD path".
//
// This is the one hand-written part of the API description, because nothing can
// derive purpose from a route table. It is safe to hand-write only because the
// *set* is not: the generator emits every recorded route whether or not there
// is a line here, and the journey suite names every operation that says nothing
// about itself. So a missing line is visible, and a line for an address that no
// longer exists is visible too.
//
// Write for somebody who has never seen this system and is choosing between a
// long list of capabilities — an assistant does exactly that, and picks wrong
// when two entries read alike.
var apiDocs = map[string]string{}
