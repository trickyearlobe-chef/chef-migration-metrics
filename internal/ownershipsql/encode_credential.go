// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import "strings"

// A password is not a URL, and whoever typed it should not have to know that it
// is about to be treated as one.
//
// A customer connection was refused by the SQL Server driver as "invalid URL
// format". Every visible part of it was legal — measured against the driver's own
// parser — so the cause was a character in the user or password that no URL can
// carry unescaped. Retyping it in another form was not possible: the password is
// encrypted, and the person who knew it had gone home. So it is encoded on the
// way to the driver, which is the one repair available from this side.
//
// This is a patch on a seam, not a design. The seam disappears when a connection
// is held as its parts rather than as a string to be parsed — see
// plans/database-connection-as-parts.md.

// charactersToEncode are the characters measured to stop a connection string
// being parsed as a URL, with what each becomes. ":" and "@" are deliberately
// absent: both were measured to be accepted inside a password, and encoding them
// would be a change with no cause.
var charactersToEncode = map[byte]string{
	' ':  "%20",
	'"':  "%22",
	'#':  "%23",
	'<':  "%3C",
	'>':  "%3E",
	'[':  "%5B",
	'\\': "%5C",
	']':  "%5D",
	'^':  "%5E",
	'`':  "%60",
	'{':  "%7B",
	'|':  "%7C",
	'}':  "%7D",
	'/':  "%2F",
	'?':  "%3F",
}

// encodeCredentialForURL percent-encodes the user and password of a URL-form
// connection string, leaving everything else exactly as written.
//
// Only the userinfo is touched. It is a bounded region with one meaning, whereas
// the host, the database and the vendor options belong to whoever wrote them and
// are none of this function's business.
//
// A "%" that already begins a valid escape is left alone. That is an assumption,
// and it is the same one every URL library makes: somebody who writes "%25" in a
// connection string means an escaped percent. The alternative — encoding it again
// — would corrupt a password that currently works, and the failure would look
// like a wrong password rather than a mangled one.
func encodeCredentialForURL(dsn string) string {
	scheme, rest, isURL := splitConnectionScheme(dsn)
	if !isURL {
		return dsn
	}
	userinfo := userinfoOfConnection(rest)
	if userinfo == "" {
		return dsn
	}

	var encoded strings.Builder
	for i := 0; i < len(userinfo); i++ {
		c := userinfo[i]
		if c == '%' && !beginsEscape(userinfo[i:]) {
			encoded.WriteString("%25")
			continue
		}
		if replacement, needsEncoding := charactersToEncode[c]; needsEncoding {
			encoded.WriteString(replacement)
			continue
		}
		encoded.WriteByte(c)
	}
	if encoded.String() == userinfo {
		return dsn
	}
	return scheme + "://" + encoded.String() + rest[len(userinfo):]
}

// splitConnectionScheme reports whether the string is in the URL spelling and
// returns the scheme and what follows "://".
func splitConnectionScheme(dsn string) (scheme, rest string, isURL bool) {
	before, after, found := strings.Cut(strings.TrimSpace(dsn), "://")
	if !found || before == "" || strings.ContainsAny(before, ";=& \t/@") {
		return "", "", false
	}
	return before, after, true
}

// userinfoOfConnection returns the user and password portion, bounded by the
// last "@" before any "?" — the same boundary the driver uses, so the two cannot
// disagree about where a password ends.
func userinfoOfConnection(rest string) string {
	limit := len(rest)
	if q := strings.Index(rest, "?"); q >= 0 {
		limit = q
	}
	at := strings.LastIndex(rest[:limit], "@")
	if at < 0 {
		return ""
	}
	return rest[:at]
}

// beginsEscape reports whether s starts with "%" followed by two hex digits.
func beginsEscape(s string) bool {
	return len(s) >= 3 && isHex(s[1]) && isHex(s[2])
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
