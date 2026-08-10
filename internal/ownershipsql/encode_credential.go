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

// mayAppearInACredential reports whether a byte can sit in a URL's userinfo as
// it stands. Written as what is allowed rather than what is not: a list of
// forbidden characters is a list somebody can leave something off, and a
// password with a "£" in it found exactly that hole.
//
// These are the unreserved and sub-delimiter characters of RFC 3986, plus ":",
// which separates the user from the password. Everything else — including every
// byte of a non-ASCII character — is percent-encoded.
func mayAppearInACredential(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9':
		return true
	}
	return strings.IndexByte("-._~!$&'()*+,;=:", b) >= 0
}

const hexDigits = "0123456789ABCDEF"

// encodeCredentialForURL percent-encodes the user and password of a URL-form
// connection string, leaving everything else exactly as written.
//
// Only the userinfo is touched. It is a bounded region with one meaning, whereas
// the host, the database and the vendor options belong to whoever wrote them and
// are none of this function's business.
//
// Every "%" in a credential is a percent sign, never an escape.
//
// Percent-encoding is a URL idea. Nobody typing a database password is thinking
// in URLs — they paste a user and a password and expect them to be used. So
// "%25" is three characters somebody typed, and "%41" is three more; neither is
// an instruction to this code.
//
// The alternative reading costs more than it saves. A pasted password containing
// "%41" parses perfectly as a URL, is decoded to "A", and the driver then
// authenticates with a password nobody typed — the login is refused, it reads as
// a bad credential, and the search goes to the account rather than the tooling.
// Against that, the population this would inconvenience is somebody who
// deliberately percent-encoded a password before pasting it, which is not a
// thing people do.
//
// It is a real trade, though: a stored connection whose password contains "%"
// and works today works because it is being decoded, and after this it is sent
// as written. That is the one case this breaks, and it breaks it loudly, as a
// refused login.
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
		if mayAppearInACredential(c) {
			encoded.WriteByte(c)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(hexDigits[c>>4])
		encoded.WriteByte(hexDigits[c&0x0F])
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
	if at := strings.LastIndex(rest[:limit], "@"); at >= 0 {
		return rest[:at]
	}
	// No "@" before the first "?", which means the "?" is inside the password
	// rather than starting the options — a password containing one hides the
	// credential from the rule above, and nothing gets encoded at all. Falling
	// back to the last "@" anywhere finds it.
	//
	// The trade is a connection whose *options* contain an "@" and whose password
	// contains a "?", where this would take too much. That needs both at once,
	// and the first branch covers every string that has an "@" where one belongs.
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		return rest[:at]
	}
	return ""
}
