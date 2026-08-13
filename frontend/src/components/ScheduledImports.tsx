// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import {
  fetchImportMappings,
  runImportNow,
  previewClearImportedOwnership,
  clearImportedOwnership,
  ApiError,
  type ClearedOwnership,
} from "../api";
import type { IntakeMapping } from "../types";
import { CronDescription } from "./CronDescription";
import { ExportButton } from "./ExportButton";
import { ErrorAlert } from "./Feedback";

// ---------------------------------------------------------------------------
// Saved database imports: what is automated, whether it is working, and the
// tools for judging a source.
//
// The loop this serves is: import a source, look at what arrived, throw it
// away, adjust the query, go again — then hand the source's owner a list of
// what is wrong with their data. So a run button, a clear-down and two reports
// live together here rather than being scattered across the app.
//
// File imports are absent by design: they have no stored source, so there is
// nothing to re-run and nothing to schedule.
// ---------------------------------------------------------------------------

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return fallback;
}

export function ScheduledImports() {
  const [imports, setImports] = useState<IntakeMapping[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await fetchImportMappings();
      setImports(res.data ?? []);
      setError(null);
    } catch {
      // Not an empty list: an unreadable list and "nothing is saved" read the
      // same on screen and mean opposite things.
      setImports(null);
      setError("Could not load the saved imports.");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (error) return <ErrorAlert message={error} />;
  if (imports === null) return null;

  const databaseImports = imports.filter((m) => m.source_kind === "database");

  return (
    <div className="space-y-6">
      {databaseImports.length === 0 ? (
        <div className="rounded-lg border border-gray-200 bg-white p-6 text-center">
          <p className="text-sm text-gray-600">No database import is saved.</p>
          <p className="mt-1 text-sm text-gray-500">
            Set one up on the “File or database” tab: choose a database source,
            map its columns, then save it. A saved import can be re-run on
            demand, or left to run on a schedule.
          </p>
        </div>
      ) : (
        <div className="table-container">
          <table className="table">
            <thead>
              <tr>
                <th>Import</th>
                <th>Runs</th>
                <th>Last run</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {databaseImports.map((m) => (
                <SavedImportRow key={m.id} mapping={m} onRan={load} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <ClearImportedOwnership onCleared={load} />
      <SourceDataReports />
    </div>
  );
}

function SavedImportRow({
  mapping,
  onRan,
}: {
  mapping: IntakeMapping;
  onRan: () => void;
}) {
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<string | null>(null);
  const [failure, setFailure] = useState<string | null>(null);

  async function run() {
    setRunning(true);
    setResult(null);
    setFailure(null);
    try {
      const res = await runImportNow(mapping.id);
      setResult(res.detail);
      // The row's last-run column is now stale — the run just wrote it.
      onRan();
    } catch (err: unknown) {
      setFailure(errorMessage(err, "The import failed."));
    } finally {
      setRunning(false);
    }
  }

  return (
    <tr>
      <td>
        <span className="font-medium text-gray-800">{mapping.name}</span>
        {mapping.db_connection && (
          <span className="block text-xs text-gray-500">
            via {mapping.db_connection}
          </span>
        )}
      </td>
      <td>
        {mapping.schedule_enabled && mapping.schedule ? (
          <>
            <code className="text-xs text-gray-700">{mapping.schedule}</code>
            <CronDescription expression={mapping.schedule} />
          </>
        ) : (
          <span className="text-sm text-gray-500">Not scheduled</span>
        )}
      </td>
      <td className="whitespace-normal">
        <LastRun mapping={mapping} />
        {result && <p className="mt-1 text-xs text-green-700">{result}</p>}
        {failure && <p className="mt-1 text-xs text-red-700">{failure}</p>}
      </td>
      <td>
        <button
          type="button"
          onClick={run}
          disabled={running}
          className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {running ? "Running…" : "Run now"}
        </button>
      </td>
    </tr>
  );
}

// LastRun says what happened, or says plainly that nothing has.
//
// A schedule that has never fired must not read like one that succeeded: those
// are the two states somebody is actually trying to tell apart when they open
// this, and a blank cell answers neither.
function LastRun({ mapping }: { mapping: IntakeMapping }) {
  if (!mapping.last_run_at) {
    return <span className="text-sm text-gray-500">Has not run yet.</span>;
  }

  const when = new Date(mapping.last_run_at).toLocaleString();
  const failed = mapping.last_run_status === "failed";

  return (
    <div className="text-sm">
      <span className={failed ? "font-medium text-red-700" : "text-green-700"}>
        {failed ? "Failed" : "Succeeded"}
      </span>{" "}
      <span className="text-gray-500">{when}</span>
      {mapping.last_run_detail && (
        <p className={failed ? "text-xs text-red-700" : "text-xs text-gray-500"}>
          {mapping.last_run_detail}
        </p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Throwing away what imports brought in
// ---------------------------------------------------------------------------

// ClearImportedOwnership removes what imports created, so the next trial import
// is judged on its own rather than against the residue of the last.
//
// Two steps, with the counts read from the server in between. Asking somebody
// to agree to "delete imported ownership" and letting them find out the size
// afterwards is how a demo database and a real one get treated the same way.
function ClearImportedOwnership({ onCleared }: { onCleared: () => void }) {
  const [preview, setPreview] = useState<ClearedOwnership | null>(null);
  const [done, setDone] = useState<ClearedOwnership | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function ask() {
    setError(null);
    setDone(null);
    try {
      setPreview(await previewClearImportedOwnership());
    } catch (err: unknown) {
      setError(errorMessage(err, "Could not work out what an import has brought in."));
    }
  }

  async function confirm() {
    setBusy(true);
    setError(null);
    try {
      const result = await clearImportedOwnership();
      setDone(result);
      setPreview(null);
      onCleared();
    } catch (err: unknown) {
      setError(errorMessage(err, "Could not remove the imported ownership."));
    } finally {
      setBusy(false);
    }
  }

  const nothingToRemove =
    preview !== null && preview.assignments === 0 && preview.owners === 0;

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4">
      <h3 className="text-sm font-semibold text-gray-800">
        Start again with a source
      </h3>
      <p className="mt-1 text-sm text-gray-500">
        Removes the ownership imports created, so the next import can be judged
        on its own. Owners and assignments added by hand are kept, as are
        aliases, rejected duplicate pairs and the failure register.
      </p>

      {error && (
        <div className="mt-3">
          <ErrorAlert message={error} />
        </div>
      )}

      {done && (
        <p className="mt-3 text-sm text-green-700">
          Removed {done.assignments} assignments and {done.owners} owners.
        </p>
      )}

      {preview === null ? (
        <button
          type="button"
          onClick={ask}
          className="mt-3 rounded-md border border-red-300 bg-white px-3 py-1.5 text-sm font-medium text-red-700 shadow-sm hover:bg-red-50"
        >
          Clear imported ownership…
        </button>
      ) : nothingToRemove ? (
        <div className="mt-3 rounded-md border border-gray-200 bg-gray-50 p-3">
          <p className="text-sm text-gray-700">
            Nothing to remove — no ownership in this instance came from an
            import.
          </p>
          <button
            type="button"
            onClick={() => setPreview(null)}
            className="mt-2 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
          >
            Cancel
          </button>
        </div>
      ) : (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-3">
          <p className="text-sm text-red-800">
            This will remove <strong>{preview.assignments}</strong> imported
            assignments and <strong>{preview.owners}</strong> owners that only
            an import ever created. It cannot be undone.
          </p>
          <div className="mt-2 flex gap-2">
            <button
              type="button"
              onClick={confirm}
              disabled={busy}
              className="rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {busy ? "Removing…" : "Yes, remove them"}
            </button>
            <button
              type="button"
              onClick={() => setPreview(null)}
              className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Reports for whoever maintains the source system
// ---------------------------------------------------------------------------

// SourceDataReports offers the two exports side by side, because the mistake to
// avoid is taking one thinking it is the other.
function SourceDataReports() {
  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4">
      <h3 className="text-sm font-semibold text-gray-800">
        Reports for whoever maintains the source
      </h3>
      <p className="mt-1 text-sm text-gray-500">
        Corrections made here do not travel back to the system the ownership
        came from, so the next import brings the same faults again. These two
        are what somebody needs in order to fix it at source.
      </p>

      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <div className="rounded-md border border-gray-200 p-3">
          <p className="text-sm font-medium text-gray-700">What to fix</p>
          <p className="mt-1 mb-2 text-xs text-gray-500">
            Duplicate people we merged, owners we reassigned, details we
            corrected — each with what the source says and what it should say.
            Plus every row the last import could not use, with the row number
            and what was wrong with it.
          </p>
          <ExportButton
            exportType="ownership_corrections"
            label="Export corrections"
          />
        </div>

        <div className="rounded-md border border-gray-200 p-3">
          <p className="text-sm font-medium text-gray-700">
            What it should look like
          </p>
          <p className="mt-1 mb-2 text-xs text-gray-500">
            Every ownership assignment as it stands now, whatever its origin —
            the shape the source data should be corrected to match.
          </p>
          <ExportButton exportType="ownership" label="Export ownership" />
        </div>
      </div>
    </div>
  );
}
