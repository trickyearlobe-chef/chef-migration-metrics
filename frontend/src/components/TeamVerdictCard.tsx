// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  fetchFailureRegisterHistory,
  recordFailureVerdict,
  resolveFailureEntry,
} from "../api";
import { useAuth } from "../context/AuthContext";
import { FailureEntryDialog } from "./FailureEntryDialog";
import type { FailureRegisterEntry } from "../types";

interface Props {
  gitRepoName: string;
  /** Defaults the label when recording. One cookbook per repo is the
   * assumption throughout; a mono-repo is deliberately not handled yet. */
  cookbookName?: string;
}

/**
 * The team's verdict, sitting beside the CookStyle and Test Kitchen cards.
 *
 * This is where a verdict gets recorded, because this is where the evidence
 * for it is: you can see what both scans say and overrule them in the same
 * glance. Recording from a list somewhere else means typing a repo name and
 * hoping it matches.
 *
 * The scans' own cards keep saying what the scans said — the disagreement is
 * meant to stay visible. This card is the one that wins.
 */
export function TeamVerdictCard({ gitRepoName, cookbookName }: Props) {
  const { isOperator } = useAuth();

  const [history, setHistory] = useState<FailureRegisterEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [recording, setRecording] = useState(false);
  const [reversing, setReversing] = useState<FailureRegisterEntry | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    return fetchFailureRegisterHistory(gitRepoName)
      .then((r) => setHistory(r.data ?? []))
      .catch((e: unknown) =>
        setError(
          e instanceof Error ? e.message : "Failed to read the team's verdict.",
        ),
      )
      .finally(() => setLoading(false));
  }, [gitRepoName]);

  useEffect(() => {
    void load();
  }, [load]);

  const standing = history.find((e) => e.status === "open") ?? null;
  const past = history.filter((e) => e.status !== "open");

  async function handleResolve() {
    if (!standing) return;
    const note = window.prompt("What was done? (optional)", "");
    if (note === null) return;
    try {
      await resolveFailureEntry(standing.id, note);
      void load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to resolve.");
    }
  }

  return (
    <div className="card" data-testid="team-verdict-card">
      <div className="mb-3 flex items-center justify-between gap-2">
        <h4 className="text-sm font-semibold text-gray-700">Team verdict</h4>
        <Link
          to="/failure-register"
          className="text-xs font-medium text-blue-600 hover:underline"
        >
          Register →
        </Link>
      </div>

      {loading ? (
        <p className="text-xs text-gray-500">Loading…</p>
      ) : error ? (
        <p className="text-xs text-red-700">{error}</p>
      ) : standing ? (
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="text-xs text-gray-500">Status:</span>
            <span
              className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
                standing.verdict === "broken"
                  ? "bg-red-100 text-red-800"
                  : "bg-green-100 text-green-800"
              }`}
            >
              {standing.verdict === "broken" ? "Broken" : "Not broken"}
            </span>
          </div>

          {/* The reason is the point. Without it the verdict is an opinion. */}
          <p className="text-xs text-gray-700">{standing.reason}</p>

          {standing.verdict === "not_broken" && (
            <p className="rounded-md border border-green-200 bg-green-50 px-2 py-1 text-xs text-green-800">
              This overrules the scans above. Nodes running it are not blocked
              by them.
            </p>
          )}
          {standing.verdict === "broken" && (
            <p className="rounded-md border border-red-200 bg-red-50 px-2 py-1 text-xs text-red-800">
              Recorded by a person. Nodes running it are blocked whatever the
              scans above say.
            </p>
          )}

          {standing.plan && (
            <p className="text-xs text-gray-600">
              <span className="font-medium text-gray-700">Plan: </span>
              {standing.plan}
            </p>
          )}
          {standing.holder_ref && (
            <p className="text-xs text-gray-600">
              <span className="font-medium text-gray-700">On it: </span>
              {standing.holder_ref}
            </p>
          )}

          <p className="text-xs text-gray-400">
            {standing.raised_by} ·{" "}
            {new Date(standing.raised_at).toLocaleDateString()}
          </p>

          {isOperator && (
            <div className="flex flex-wrap gap-2 pt-1">
              <button
                type="button"
                onClick={() => setReversing(standing)}
                className="rounded-md border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50"
              >
                Change verdict
              </button>
              <button
                type="button"
                onClick={() => void handleResolve()}
                className="rounded-md bg-blue-600 px-2 py-1 text-xs font-medium text-white hover:bg-blue-700"
              >
                Resolve
              </button>
            </div>
          )}
        </div>
      ) : (
        <div className="space-y-2">
          <p className="text-xs text-gray-500">
            Nobody has recorded a verdict on this repo.
          </p>
          {isOperator && (
            <button
              type="button"
              onClick={() => setRecording(true)}
              className="rounded-md bg-blue-600 px-2 py-1 text-xs font-medium text-white hover:bg-blue-700"
            >
              Report this as broken — or not
            </button>
          )}
        </div>
      )}

      {/* Superseded and resolved verdicts stay readable: a scan called this
          incompatible, a person overruled it, and why. */}
      {past.length > 0 && (
        <details className="mt-3 border-t border-gray-100 pt-2">
          <summary className="cursor-pointer text-xs text-gray-500">
            {past.length} earlier verdict{past.length === 1 ? "" : "s"}
          </summary>
          <ul className="mt-2 space-y-2">
            {past.map((e) => (
              <li key={e.id} className="text-xs text-gray-500">
                <span className="font-medium">
                  {e.verdict === "broken" ? "Broken" : "Not broken"}
                </span>{" "}
                — {e.reason}
                <span className="block text-gray-400">
                  {e.raised_by} ·{" "}
                  {e.status === "resolved" ? "resolved" : "overturned"}
                </span>
              </li>
            ))}
          </ul>
        </details>
      )}

      {recording && (
        <FailureEntryDialog
          mode="record"
          fixedRepo={{ name: gitRepoName, cookbookName }}
          onCancel={() => setRecording(false)}
          onSubmit={async (body) => {
            await recordFailureVerdict(body);
            setRecording(false);
            void load();
          }}
        />
      )}

      {reversing && (
        <FailureEntryDialog
          mode="reverse"
          entry={reversing}
          onCancel={() => setReversing(null)}
          onSubmit={async (body) => {
            await recordFailureVerdict(body);
            setReversing(null);
            void load();
          }}
        />
      )}
    </div>
  );
}
