import { useEffect, useState } from "react";
import {
  PASSWORD_MARKER,
  deleteOwnershipConnection,
  fetchCredentials,
  listOwnershipConnections,
  saveOwnershipConnection,
  showOwnershipConnection,
  testOwnershipConnection,
  type ComposedConnection,
  type ConnectionTestResult,
  type OwnershipConnection,
} from "../api";

// Setting up the connection an import reads through.
//
// See journeys/ownership-connection.md. The whole point of this panel is that
// the connection is READABLE: the address, the database, the account and the
// domain are on screen and editable, and only the password is out of sight.
// Hiding the rest to protect that one part is what made a fortnight of failures
// impossible to diagnose.

const INPUT =
  "block w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500";

// A connection that would work, to start from — not one imposed. Every one is
// editable the moment it appears, and a string from somebody else's tooling can
// be pasted over it whole.
//
// The transport options are in the proposal rather than in a control of their
// own, and they are not guesses: each pair was measured against a real server
// (internal/ownershipsql/tls_mode.go). "encrypt=false" and saying nothing about
// encryption parse identically and then behave differently, which is exactly
// the kind of thing nobody should have to discover twice.
// Joined rather than written as one string so that no line in this file has the
// shape of a real connection with a password in it. The pre-commit scanner
// blocks that shape and is right to — a proposal is not worth teaching anybody
// to reach for --no-verify.
const PROPOSED: Record<string, string> = {
  sqlserver:
    "sqlserver://user:" + PASSWORD_MARKER + "@host:1433?database=cmdb&encrypt=true&TrustServerCertificate=true",
  postgres: "postgres://user:" + PASSWORD_MARKER + "@host:5432/cmdb?sslmode=require",
};

const TRANSPORT_NOTES: Record<string, string> = {
  sqlserver:
    "encrypt=true with TrustServerCertificate=true encrypts without checking the certificate — which is what a server using SQL Server's own self-signed certificate needs. Drop TrustServerCertificate to check it, encrypt=strict for TDS 8.0, or encrypt=disable for no encryption at all.",
  postgres:
    "sslmode=require encrypts without checking the certificate. Use verify-full to check it, or disable for a server with no TLS — which sends the password across the network in the clear. Leaving sslmode out is not the same as disable: the driver then demands TLS and fails against a server without it.",
};

// What each outcome means and who it belongs to. Five different answers, each a
// different person to go and talk to — which is the reason a connection test is
// its own act rather than a side effect of asking for the table list.
const OUTCOMES: Record<string, { label: string; whose: string; good?: boolean }> = {
  connected: { label: "Connected", whose: "The server answered and this is usable.", good: true },
  malformed: {
    label: "The connection could not be read",
    whose: "Nothing was dialled. This one is ours to fix — read the composed connection above.",
  },
  unreachable: {
    label: "Nothing answered",
    whose: "A wrong address, a closed port or a firewall. Somebody in networking.",
  },
  refused: {
    label: "The account was refused",
    whose: "The server answered and rejected the account or its password. Whoever owns the account.",
  },
  "no-database": {
    label: "No such database",
    whose: "The login worked and the database named did not. Whoever knows that server.",
  },
  "untrusted-domain": {
    label: "The account is not the database's to check",
    whose:
      "Anything before a backslash — a domain, a machine name, a workgroup or a dot — hands the login to a directory instead. This server is not in it, or cannot reach it. Whoever runs that directory. Note this refusal does not name the account back, so the composed connection above is all there is to check.",
  },
  unknown: {
    label: "Refused in words we do not recognise",
    whose: "The server's own words are below. Naming the wrong team would be worse than saying this.",
  },
};

export function OwnershipConnectionPanel({
  value,
  onChange,
}: {
  value: string;
  onChange: (name: string) => void;
}) {
  const [connections, setConnections] = useState<OwnershipConnection[]>([]);
  const [listError, setListError] = useState<string | null>(null);
  const [credentialNames, setCredentialNames] = useState<string[]>([]);
  const [credentialError, setCredentialError] = useState<string | null>(null);

  // The connection being written. Editing an existing one loads it here, so
  // what is on screen is always the thing that will be sent.
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState("");
  const [driver, setDriver] = useState("sqlserver");
  const [connection, setConnection] = useState("");
  const [passwordCredential, setPasswordCredential] = useState("");

  const [composed, setComposed] = useState<ComposedConnection | null>(null);
  const [result, setResult] = useState<ConnectionTestResult | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    listOwnershipConnections()
      .then((res) => {
        if (cancelled) return;
        setConnections(res.data ?? []);
        setListError(null);
      })
      .catch(() => {
        if (cancelled) return;
        // An unreadable list must not render as an empty one: they read the
        // same on screen and mean opposite things.
        setConnections([]);
        setListError("Could not load the connections that have been set up.");
      });
    return () => {
      cancelled = true;
    };
  }, [saved]);

  useEffect(() => {
    if (!editing) return;
    let cancelled = false;
    fetchCredentials()
      .then((res) => {
        if (cancelled) return;
        setCredentialNames((res.data ?? []).map((c) => c.name));
        setCredentialError(null);
      })
      .catch(() => {
        if (cancelled) return;
        setCredentialNames([]);
        setCredentialError("Could not load the stored credentials.");
      });
    return () => {
      cancelled = true;
    };
  }, [editing]);

  // A URL-shaped connection already says which database it is for, so it is not
  // asked for twice. A keyword-shaped one carries no scheme, so it still has to
  // be said.
  const schemeNamesDatabase = /^\s*(sqlserver|postgres|postgresql):\/\//i.test(connection);

  function startNew() {
    setEditing(true);
    setName("");
    setDriver("sqlserver");
    setConnection(PROPOSED.sqlserver);
    setPasswordCredential("");
    setComposed(null);
    setResult(null);
    setError(null);
    setSaved(null);
  }

  function startEditing(existing: OwnershipConnection) {
    setEditing(true);
    setName(existing.name);
    setDriver(existing.driver);
    setConnection(existing.connection);
    setPasswordCredential(existing.password_credential);
    setComposed(null);
    setResult(null);
    setError(null);
    setSaved(null);
  }

  function chooseDriver(next: string) {
    setDriver(next);
    // Only when nothing has been written over the proposal — an edited
    // connection is the administrator's, and replacing it would be exactly the
    // quiet rewriting this screen exists to stop.
    if (connection === "" || Object.values(PROPOSED).includes(connection)) {
      setConnection(PROPOSED[next] ?? "");
    }
  }

  function reportError(err: unknown, fallback: string) {
    setError(err instanceof Error && err.message ? err.message : fallback);
  }

  async function handleShow() {
    setError(null);
    setBusy("Composing…");
    try {
      setComposed(await showOwnershipConnection({ driver, connection }));
    } catch (err: unknown) {
      setComposed(null);
      reportError(err, "Could not compose that connection.");
    } finally {
      setBusy(null);
    }
  }

  async function handleTest() {
    setError(null);
    setResult(null);
    setBusy("Asking the server…");
    try {
      const answer = await testOwnershipConnection({
        driver,
        connection,
        password_credential: passwordCredential,
      });
      setResult(answer);
      // The test answers with what it actually sent, so the masked connection
      // on screen is the one the server saw rather than one composed separately.
      if (answer.connection) {
        setComposed({ driver, connection: answer.connection, form: answer.form });
      }
    } catch (err: unknown) {
      reportError(err, "Could not test that connection.");
    } finally {
      setBusy(null);
    }
  }

  async function handleSave() {
    setError(null);
    setBusy("Keeping it…");
    try {
      const stored = await saveOwnershipConnection({
        name,
        driver,
        connection,
        password_credential: passwordCredential,
      });
      setSaved(stored.name);
      setEditing(false);
      onChange(stored.name);
    } catch (err: unknown) {
      reportError(err, "Could not keep that connection.");
    } finally {
      setBusy(null);
    }
  }

  async function handleDelete(target: string) {
    setError(null);
    try {
      await deleteOwnershipConnection(target);
      if (value === target) onChange("");
      setSaved(`removed ${target}`);
    } catch (err: unknown) {
      reportError(err, "Could not remove that connection.");
    }
  }

  const chosen = connections.find((c) => c.name === value);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <label className="text-sm text-gray-700">
          <span className="mb-1 block text-xs font-medium text-gray-500">Connection</span>
          <select
            aria-label="Connection"
            value={value}
            onChange={(e) => onChange(e.target.value)}
            className="min-w-56 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm"
          >
            <option value="">Choose a connection…</option>
            {connections.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
          </select>
        </label>

        <button
          type="button"
          onClick={startNew}
          className="rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
        >
          Set up a new one
        </button>
        {chosen && !editing && (
          <>
            <button
              type="button"
              onClick={() => startEditing(chosen)}
              className="rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
            >
              Edit
            </button>
            <button
              type="button"
              onClick={() => handleDelete(chosen.name)}
              className="rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-red-700 hover:bg-red-50"
            >
              Remove
            </button>
          </>
        )}
      </div>

      {listError && <p className="text-xs text-red-600">{listError}</p>}

      {/* What the chosen connection sends, without opening it for editing. */}
      {chosen && !editing && (
        <p className="break-all font-mono text-xs text-gray-700">{chosen.connection}</p>
      )}

      {editing && (
        <div className="space-y-3 rounded-md border border-gray-200 bg-gray-50 p-3">
          <div className="flex flex-wrap items-end gap-3">
            <label className="text-sm text-gray-700">
              <span className="mb-1 block text-xs font-medium text-gray-500">Name</span>
              <input
                aria-label="Connection name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="asset-database"
                className={INPUT}
              />
            </label>

            {/* Asked only when the connection does not already answer it. */}
            {!schemeNamesDatabase && (
              <label className="text-sm text-gray-700">
                <span className="mb-1 block text-xs font-medium text-gray-500">Database</span>
                <select
                  aria-label="Database"
                  value={driver}
                  onChange={(e) => chooseDriver(e.target.value)}
                  className="rounded-md border border-gray-300 px-2.5 py-1.5 text-sm"
                >
                  <option value="sqlserver">SQL Server</option>
                  <option value="postgres">PostgreSQL</option>
                </select>
              </label>
            )}

            <label className="text-sm text-gray-700">
              <span className="mb-1 block text-xs font-medium text-gray-500">Password</span>
              <select
                aria-label="Password credential"
                value={passwordCredential}
                onChange={(e) => setPasswordCredential(e.target.value)}
                className="min-w-56 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm"
              >
                <option value="">Choose the stored credential…</option>
                {credentialNames.map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <label className="block text-sm text-gray-700">
            <span className="mb-1 block text-xs font-medium text-gray-500">
              The connection, as it will be sent
            </span>
            <textarea
              aria-label="Connection string"
              value={connection}
              onChange={(e) => setConnection(e.target.value)}
              rows={3}
              spellCheck={false}
              className={`${INPUT} font-mono text-xs`}
            />
          </label>

          <p className="text-xs text-gray-500">
            Write <code className="font-mono text-gray-800">{PASSWORD_MARKER}</code> where the
            password goes, and it is put in for you — escaped for the form this connection is
            written in. It is the one value you never see, so it is the one you can never check;
            everything else here stays exactly as you typed it. Leave the marker out and this is
            refused rather than guessed at.
          </p>
          <p className="text-xs text-gray-500">{TRANSPORT_NOTES[driver]}</p>
          {credentialError ? (
            <p className="text-xs text-red-600">{credentialError}</p>
          ) : (
            <p className="text-xs text-gray-500">
              The password is a credential, added under Admin → Credentials as type Generic. It
              never travels through this screen.
            </p>
          )}

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={handleShow}
              disabled={busy !== null || connection.trim() === ""}
              className="rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              Show what will be sent
            </button>
            <button
              type="button"
              onClick={handleTest}
              disabled={busy !== null || connection.trim() === "" || passwordCredential === ""}
              className="rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              Test it
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={busy !== null || name.trim() === "" || connection.trim() === ""}
              className="rounded-md bg-blue-600 px-2.5 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              Keep it
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              className="px-2.5 py-1.5 text-sm text-gray-600 hover:text-gray-800"
            >
              Cancel
            </button>
            {busy && <span className="text-xs text-gray-500">{busy}</span>}
          </div>

          {error && <p className="text-xs text-red-600">{error}</p>}

          {/* The answer to the question that has cost days: what was actually
              sent. Masked, so it can be screenshotted and pasted into a ticket. */}
          {composed && (
            <div className="rounded-md border border-gray-200 bg-white px-3 py-2">
              <p className="text-xs text-gray-500">
                This is what will be sent, read as {composed.form === "url" ? "a URL" : `the ${composed.form} form`}:
              </p>
              <code className="mt-1 block select-all break-all font-mono text-xs text-gray-800">
                {composed.connection}
              </code>
            </div>
          )}

          {result && (
            <div
              className={`rounded-md border px-3 py-2 ${
                OUTCOMES[result.outcome]?.good
                  ? "border-green-200 bg-green-50"
                  : "border-amber-200 bg-amber-50"
              }`}
            >
              <p className="text-sm font-medium text-gray-800">
                {OUTCOMES[result.outcome]?.label ?? result.outcome}
              </p>
              <p className="mt-0.5 text-xs text-gray-600">{OUTCOMES[result.outcome]?.whose}</p>
              {result.detail && (
                <p className="mt-1 break-all font-mono text-xs text-gray-700">{result.detail}</p>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
