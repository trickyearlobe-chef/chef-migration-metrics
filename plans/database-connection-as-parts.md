# Hold a database connection as its parts, not as a string to be parsed

## The question that produced this

"Is this all because we are using url parsing instead of constructing a url from
creds, and fqdn and some optional params?"

Largely yes. Every failure over one evening was the gap between two parsers —
ours and the driver's. A connection that satisfied ours was refused by the
driver, or the reverse, and each fix narrowed the gap rather than removing it.

## What a stored connection is today

One opaque encrypted string, typed or pasted by a person. Everything else is
inference: whether it names a database, which driver it implies, which part is
the password, whether it can be parsed at all.

## What went wrong because of that

- Options appended with `;` rather than `?` — refused, though the driver accepts
  the `?` spelling. A customer was blocked mid-session.
- `%`, a space, `#`, `/` or `?` in a password, or `\` in a username: each makes a
  URL unparsable. The driver refuses them; nothing else can be done about it.
- A named instance (`host\INSTANCE`) cannot be written as a URL at all.
- `sslmode` absent means "TLS required" to lib/pq, stricter than psql, so a
  working connection fails and the error never mentions the connection string.
- Redaction had to guess where the credential ended — the last `@` before any
  `?` — and an early version leaked a password containing `/`.
- A format was displayed on screen and then refused, more than once, because the
  example and the validator were separate things that could disagree.

## The shape that removes them

Store the parts: host, port, instance, database, user, password, and a free-form
list of vendor options. Construct the connection string when connecting, choosing
the spelling that can carry what is there — keyword-value where a URL cannot.

Consequences, in order of how much they matter:

1. **Only the password is a secret.** Host, database and options are not. Today
   the whole string is one secret, so it is encrypted, so it cannot be read, so
   it needs a redactor, so it needs a shape describer. That chain is where the
   evening went. Split it and a log can name every field except one, exactly,
   with no inference and no redactor.
2. **Nothing is parsed on the way in, so nothing can be refused for its
   spelling.** A `%` in a password stops being a problem the moment we are the
   ones writing the string.
3. **The driver is a field, not a guess.** This absorbs the separate backlog item
   about deriving it from the scheme.
4. **TLS is a field with a default and a control**, not a rewrite of somebody's
   string.

## What must not be lost

**A DBA hands over a connection string.** That is the real workflow — see
`journeys/ownership-intake.md`: the person configuring the import usually cannot
inspect the database and did not compose the connection. So keep parsing, and
demote it: paste a string to pre-fill the fields, editable, and if it cannot be
parsed the fields are filled in by hand instead. Parsing as a convenience is
fine. It was only ever wrong as a gate.

**Vendor options are open-ended** — `ApplicationIntent`, `MultiSubnetFailover`,
`failoverPartner`, and whatever the next estate uses. They stay a list that is
passed through and never adjudicated, which is the discipline this code kept
failing at.

## Where the customer's values actually come from

Reported after the evening this plan came out of, and it changes the target
rather than the reasoning.

The host and the credential are held in HashiCorp Vault. Something authenticates
to Vault using the Chef client key, extracts the values into environment
variables, and a Python script uses them directly. Nothing in that chain
percent-encodes anything, which settles the question this plan was written
around: the values are literal, and a "%" in a password is a percent sign.

It also says the parts already exist as parts, and are only assembled into a
string because our screen asks for one. Two consequences worth weighing when this
is picked up:

- **Reading them from Vault directly is the better end state.** The credential
  would never be pasted, never be typed, and never be re-encrypted by us. The
  authentication path already exists, since the Chef client key is a credential
  this tool already holds.
- **Whatever their Python library expects is the dialect they know.** Worth
  finding out before choosing what the screen accepts — matching the form they
  already have beats teaching them a new one.

## Migration

Existing credentials are strings and must keep working, so the parser stays for
reading them. One-time: offer the parsed fields for confirmation when a stored
connection is next edited. Nothing forces a rewrite.

## Test first

That a connection built from parts carrying a `%` in the password, a named
instance, and two vendor options connects to a real server — against the local
SQL Server container, which is what settled the TLS mapping. Then that the log
line names host, database and options and omits only the password, with no
redaction step involved.

## Absorbs

The backlog item on deriving the driver from the connection string. That question
disappears here rather than being answered.
