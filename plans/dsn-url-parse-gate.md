# Stop URL parsing deciding whether a connection string is usable

A customer's colleague was refused with "value is not a database connection for a
supported driver (postgres, sqlserver)" on a `sqlserver://` string. The scheme was
right. The refusal came from `url.Parse` failing, reported as a driver problem.

## What is actually wrong

`validateDatabaseURL` (`internal/secrets/database_url.go`) uses `url.Parse` as a
gate. Four shapes that are ordinary in a SQL Server estate all fail it:

- params separated by `;` directly after the host, with no `?` — "invalid port"
- a named instance, `host\INSTANCE` — backslash illegal in a URL host
- a Windows-auth user, `DOMAIN\svc` — "invalid userinfo"
- a bare `%` in a password — "invalid URL escape"

Each is reported as an unsupported driver, which sends the reader to the wrong
place entirely. Measured, not reasoned about.

Two faults behind it:

1. **Nothing here needs a parsed URL.** The question is whether a database is
   named, and `namesDatabase` already answers it textually on the raw string. It
   returns the right answer for all four shapes above.
2. **The scheme check re-derives a settled decision.** The handler validates
   `db_driver` against the supported list before reading the credential
   (`handle_ownership_intake.go`), so which driver to open is already known. A
   scheme disagreeing with it can only block, never inform — which is how
   `jdbc:sqlserver://` and `mssql://` get refused too.

## Decided

- Derive the scheme textually, never from `url.Parse`. A parse failure decides
  nothing.
- Extract a Postgres path-form database textually.
- **The accepted schemes are the ones the screen shows, and no others.** The
  check exists to catch something missing from a format we offered, not to
  recognise formats nobody was given.
- Keep both refusals that earn their place: something that is not a connection
  string at all, and one that names no database.
- When refusing, say the actual cause, and point at the keyword-value form —
  which is the only form that can carry a named instance or a domain user.

## Test first

`TestDatabaseURL_AcceptsTheShapesURLParsingCannotHandle`, pinning the four shapes
by literal text rather than paraphrase, because the paraphrase is what let this
through twice.

The existing refusal tests must stay green: a value naming no database, and a
driver we cannot open, are still refused.

## Not in scope

Whether the driver can then open the string. It cannot be known here, and the
driver's own error is more use than a guess.
