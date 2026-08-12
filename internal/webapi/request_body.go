// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

// Reading what a caller sent.
//
// Go's JSON decoder ignores fields it does not recognise. That default made
// every write call here quietly forgiving: a caller could misspell a field, or
// send one this service never had, and be told the call worked — after which
// neither side could say what had actually been acted on. Three of those were
// live when this was written, one of them a settings box an administrator could
// type into that had never reached anything.
//
// So a body is refused when it carries something this service cannot
// understand, and the refusal names the field. Naming it is the part that
// matters: a caller told "invalid JSON" sends the same body again, and one told
// which field is wrong fixes it. That difference is the whole of the gap
// between a caller correcting itself and a caller insisting — which is why it
// arrived alongside the assistant surface, where the caller is a program that
// built its request from our description.

// decodeJSONBody reads the request body into target, refusing a body carrying
// anything this service does not understand.
//
// Returns false when it has already written the refusal, so a handler's whole
// obligation is to stop.
func decodeJSONBody(w http.ResponseWriter, req *http.Request, target any) bool {
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		WriteBadRequest(w, requestBodyProblem(err))
		return false
	}
	return true
}

// decodeOptionalJSONBody is decodeJSONBody where sending nothing at all is a
// valid call. An empty body leaves target as it was; anything that IS sent is
// held to the same standard as anywhere else.
//
// The alternative it replaced was ignoring every decode error, which made an
// optional body a body nobody read: a note with a misspelt field resolved the
// entry silently without the note.
func decodeOptionalJSONBody(w http.ResponseWriter, req *http.Request, target any) bool {
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		WriteBadRequest(w, requestBodyProblem(err))
		return false
	}
	return true
}

// unknownFieldPattern matches what the decoder says when it meets a field the
// target type does not have.
//
// Read out of the message because the decoder reports this as a plain error
// with no type to inspect — there is nothing else to match on. If a future
// version of Go words it differently the field simply stops being named and the
// caller gets the general message, which is the behaviour that was there
// before; a test holds the naming so the change would not pass unnoticed.
//
// Two of them, because a settings section is read by the YAML decoder so that a
// caller may send either YAML or JSON, and the two word it differently.
var unknownFieldPatterns = []*regexp.Regexp{
	regexp.MustCompile(`unknown field "([^"]+)"`),       // encoding/json
	regexp.MustCompile(`field (\S+) not found in type`), // yaml.v3
}

// requestBodyProblem says what was wrong with a body, in terms a caller can act
// on rather than in the decoder's own words.
//
// The field name is lifted out and a message built round it rather than the
// decoder's own being passed through: the YAML one names the Go type it was
// decoding into, which tells a caller nothing they can use and tells anybody
// else what this service is made of.
func requestBodyProblem(err error) string {
	for _, pattern := range unknownFieldPatterns {
		if match := pattern.FindStringSubmatch(err.Error()); match != nil {
			return fmt.Sprintf(
				"This call does not accept a field called %q. Nothing was changed. Check it "+
					"against the API description, which lists what this call reads.", match[1])
		}
	}
	return "Invalid or malformed JSON request body."
}
