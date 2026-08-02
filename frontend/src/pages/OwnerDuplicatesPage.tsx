// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  dismissOwnerDuplicate,
  fetchOwnerDuplicateDismissals,
  fetchOwnerDuplicates,
  rescanOwnerDuplicates,
  restoreOwnerDuplicate,
} from "../api";
import { useAuth } from "../context/AuthContext";
import { EmptyState, ErrorAlert, LoadingSpinner } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { OwnerMergeDialog } from "../components/OwnerMergeDialog";
import type {
  OwnerDuplicateCandidate,
  OwnerDuplicateDismissal,
  OwnerDuplicatesResponse,
  Pagination as PaginationType,
} from "../types";

const PER_PAGE = 25;

const percent = (similarity: number) => `${Math.round(similarity * 100)}%`;

const MATCHED_ON_LABELS: Record<string, string> = {
  name: "owner name",
  display_name: "display name",
  alias: "alias",
};

/**
 * The people who may already be somebody else.
 *
 * The import invents owners on purpose — that is where alias candidates come
 * from — but until now the only screen that ever paired a new person with who
 * they might already be was the import report, which is React state and gone
 * on navigation.
 */
export function OwnerDuplicatesPage() {
  const { isAdmin, isOperator } = useAuth();

  const [response, setResponse] = useState<OwnerDuplicatesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [merging, setMerging] = useState<OwnerDuplicateCandidate | null>(null);
  const [merged, setMerged] = useState<string | null>(null);
  const [scanRequested, setScanRequested] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);
  const [dismissing, setDismissing] = useState<string | null>(null);
  // A rejected pair is hidden from the list above, so it needs somewhere of
  // its own to be seen — otherwise a mis-click suppresses a pair permanently
  // and invisibly, which is worse than the problem dismissing solved.
  const [showRejected, setShowRejected] = useState(false);
  const [rejected, setRejected] = useState<OwnerDuplicateDismissal[]>([]);

  const loadRejected = useCallback(() => {
    return fetchOwnerDuplicateDismissals()
      .then((r) => setRejected(r.data ?? []))
      .catch(() => setRejected([]));
  }, []);

  useEffect(() => {
    if (showRejected) void loadRejected();
  }, [showRejected, loadRejected]);

  async function handleRestore(pair: OwnerDuplicateDismissal) {
    try {
      await restoreOwnerDuplicate({
        owner_a: pair.owner_a,
        owner_b: pair.owner_b,
      });
      setMerged(
        `${pair.owner_a} and ${pair.owner_b} can be paired again. They will reappear above if the scan still finds them similar.`,
      );
      await Promise.all([loadRejected(), load()]);
    } catch (e: unknown) {
      setScanError(
        e instanceof Error ? e.message : "Failed to undo the rejection.",
      );
    }
  }

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    return fetchOwnerDuplicates({ page, per_page: PER_PAGE })
      .then(setResponse)
      .catch((e: unknown) =>
        setError(
          e instanceof Error ? e.message : "Failed to load possible duplicates.",
        ),
      )
      .finally(() => setLoading(false));
  }, [page]);

  useEffect(() => {
    void load();
  }, [load]);

  const candidates = response?.data ?? [];
  const coverage = response?.coverage ?? {};
  const pagination: PaginationType | null = response?.pagination ?? null;
  const scan = response?.scan;
  const scanRunning = (response?.scan_running ?? false) || scanRequested;
  const neverScanned = response !== null && !scan;

  // Saying "not the same person". The rejection outlives a rescan, so the list
  // can actually be worked down to nothing rather than returning intact.
  async function handleDismiss(candidate: OwnerDuplicateCandidate) {
    const key = `${candidate.owner_a}|${candidate.owner_b}`;
    const reason = window.prompt(
      `Recording that ${candidate.owner_a} and ${candidate.owner_b} are different people.\n\nWhy? (optional)`,
      "",
    );
    if (reason === null) return;
    setDismissing(key);
    try {
      await dismissOwnerDuplicate({
        owner_a: candidate.owner_a,
        owner_b: candidate.owner_b,
        reason,
      });
      setMerged(
        `${candidate.owner_a} and ${candidate.owner_b} will not be paired again.`,
      );
      await load();
    } catch (e: unknown) {
      setScanError(
        e instanceof Error ? e.message : "Failed to dismiss the pair.",
      );
    } finally {
      setDismissing(null);
    }
  }

  async function handleRescan() {
    setScanError(null);
    setScanRequested(true);
    try {
      const result = await rescanOwnerDuplicates();
      if (!result.started) {
        setScanError(result.reason ?? "A scan is already running.");
      }
    } catch (e: unknown) {
      setScanRequested(false);
      setScanError(e instanceof Error ? e.message : "Failed to start the scan.");
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-gray-800">
            Possible Duplicate Owners
          </h2>
          <p className="mt-1 text-sm text-gray-500">
            Pairs of owners that may be the same person. These are leads to
            recognise, not matches — nothing is merged until you say so.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {isOperator && (
            <button
              type="button"
              onClick={() => void handleRescan()}
              disabled={scanRunning}
              className="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm transition-colors hover:bg-blue-700 disabled:opacity-50"
            >
              {scanRunning ? "Scanning…" : "Scan for duplicates"}
            </button>
          )}
          <button
            type="button"
            onClick={() => setShowRejected((v) => !v)}
            className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
          >
            {showRejected ? "Hide rejected" : "Rejected pairs"}
            {(response?.dismissed_pairs ?? 0) > 0 &&
              ` (${response?.dismissed_pairs})`}
          </button>
          <Link
            to="/ownership"
            className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
          >
            All owners
          </Link>
        </div>
      </div>

      {/* Where the list came from. A list built an hour ago is a different
          claim from one built just now, and both differ from never. */}
      <div className="flex flex-wrap items-center gap-3 text-xs text-gray-600">
        {scanRunning ? (
          <span className="rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-blue-700">
            A scan is running. It walks every owner and every alias, so it can
            take a minute on a large catalogue — reload the page to see the
            result. Until it finishes, the list below is the previous one.
          </span>
        ) : scan ? (
          <span className="rounded-md border border-gray-200 bg-gray-50 px-3 py-2">
            Last scanned {new Date(scan.scanned_at).toLocaleString()} —{" "}
            {scan.pairs_found} pair{scan.pairs_found === 1 ? "" : "s"} found.
          </span>
        ) : (
          neverScanned && (
            <span className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-amber-800">
              This catalogue has never been scanned, so there is nothing to show
              yet — an empty list here does not mean there are no duplicates.
            </span>
          )
        )}
      </div>

      {scanError && (
        <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          {scanError}
        </div>
      )}

      {coverage.owners_total !== undefined &&
        coverage.owners_without_alias !== undefined && (
          <p className="rounded-md border border-gray-200 bg-gray-50 px-3 py-2 text-xs text-gray-600">
            Owners are compared by name, and by any alias they have.{" "}
            {coverage.owners_without_alias} of {coverage.owners_total} owners
            have no alias recorded, so those are compared by name alone.
          </p>
        )}

      {showRejected && (
        <div className="card" data-testid="rejected-pairs">
          <h3 className="card-header">Rejected pairs</h3>
          {rejected.length === 0 ? (
            <p className="text-sm text-gray-500">
              Nothing has been rejected yet.
            </p>
          ) : (
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-gray-200 text-xs font-medium uppercase tracking-wider text-gray-500">
                  <th className="px-3 py-2">Pair</th>
                  <th className="px-3 py-2">Why</th>
                  <th className="px-3 py-2">Rejected by</th>
                  {isOperator && <th className="px-3 py-2" />}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {rejected.map((pair) => (
                  <tr key={`${pair.owner_a}|${pair.owner_b}`}>
                    <td className="px-3 py-2">
                      {pair.owner_a} &amp; {pair.owner_b}
                    </td>
                    <td className="px-3 py-2 text-gray-600">
                      {pair.reason || "—"}
                    </td>
                    <td className="whitespace-nowrap px-3 py-2 text-xs text-gray-500">
                      {pair.dismissed_by}
                      <div>
                        {new Date(pair.dismissed_at).toLocaleDateString()}
                      </div>
                    </td>
                    {isOperator && (
                      <td className="px-3 py-2 text-right">
                        <button
                          type="button"
                          onClick={() => void handleRestore(pair)}
                          className="rounded-md border border-gray-300 bg-white px-3 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50"
                        >
                          Undo
                        </button>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {merged && (
        <div className="rounded-md border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-700">
          {merged}
        </div>
      )}

      {loading ? (
        <LoadingSpinner message="Looking for possible duplicates…" />
      ) : error ? (
        <ErrorAlert message={error} onRetry={() => void load()} />
      ) : candidates.length === 0 ? (
        <div className="card">
          {neverScanned ? (
            <EmptyState
              title="Not scanned yet"
              description="Run a scan to pair owners that may be the same person."
            />
          ) : (
            <EmptyState
              title="No possible duplicates found"
              description={
                (response?.dismissed_pairs ?? 0) > 0
                  ? `No pairs left to look at. ${response?.dismissed_pairs} have been rejected as different people and will not be paired again.`
                  : "No two owners look enough alike to pair. This covers owner names and every recorded alias."
              }
            />
          )}
        </div>
      ) : (
        <div className="card">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-gray-200 text-xs font-medium uppercase tracking-wider text-gray-500">
                  <th className="px-3 py-2">Owner</th>
                  <th className="px-3 py-2">Possibly the same as</th>
                  <th className="px-3 py-2">Matched on</th>
                  <th className="px-3 py-2">Similarity</th>
                  {isOperator && <th className="px-3 py-2">Action</th>}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {candidates.map((candidate) => (
                  <tr
                    key={`${candidate.owner_a}|${candidate.owner_b}`}
                    className="hover:bg-gray-50"
                  >
                    <td className="px-3 py-2">
                      <OwnerCell
                        name={candidate.owner_a}
                        value={candidate.value_a}
                        matchedOn={candidate.matched_on}
                        assignments={candidate.assignments_a}
                      />
                    </td>
                    <td className="px-3 py-2">
                      <OwnerCell
                        name={candidate.owner_b}
                        value={candidate.value_b}
                        matchedOn={candidate.matched_on}
                        assignments={candidate.assignments_b}
                      />
                    </td>
                    <td className="px-3 py-2 text-gray-600">
                      {MATCHED_ON_LABELS[candidate.matched_on] ??
                        candidate.matched_on}
                    </td>
                    <td className="px-3 py-2 font-medium text-gray-700">
                      {percent(candidate.similarity)}
                    </td>
                    {isOperator && (
                      <td className="px-3 py-2">
                        <div className="flex gap-2">
                          {isAdmin && (
                            <button
                              type="button"
                              onClick={() => {
                                setMerged(null);
                                setMerging(candidate);
                              }}
                              className="rounded-md bg-blue-600 px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-blue-700"
                            >
                              Merge…
                            </button>
                          )}
                          <button
                            type="button"
                            disabled={
                              dismissing ===
                              `${candidate.owner_a}|${candidate.owner_b}`
                            }
                            onClick={() => {
                              setMerged(null);
                              void handleDismiss(candidate);
                            }}
                            className="rounded-md border border-gray-300 bg-white px-3 py-1 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50"
                          >
                            Not a duplicate
                          </button>
                        </div>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {pagination && (
            <div className="mt-4">
              <Pagination pagination={pagination} onPageChange={setPage} />
            </div>
          )}
        </div>
      )}

      {merging && (
        <OwnerMergeDialog
          // The side holding less work folds into the side holding more, which
          // is the direction that moves the fewest assignments. Swappable.
          fromOwner={
            merging.assignments_a <= merging.assignments_b
              ? merging.owner_a
              : merging.owner_b
          }
          intoOwner={
            merging.assignments_a <= merging.assignments_b
              ? merging.owner_b
              : merging.owner_a
          }
          onCancel={() => setMerging(null)}
          onMerged={(result) => {
            setMerging(null);
            setMerged(
              `${result.from_owner} was merged into ${result.into_owner}: ` +
                `${result.reassigned} assignment(s) moved, ` +
                `${result.aliases_moved} alias(es) moved.`,
            );
            void load();
          }}
        />
      )}
    </div>
  );
}

function OwnerCell({
  name,
  value,
  matchedOn,
  assignments,
}: {
  name: string;
  value: string;
  matchedOn: string;
  assignments: number;
}) {
  return (
    <div className="space-y-0.5">
      <Link
        to={`/ownership/${encodeURIComponent(name)}`}
        className="font-medium text-blue-600 hover:underline"
      >
        {name}
      </Link>
      {matchedOn !== "name" && value !== name && (
        <div className="text-xs text-gray-500">known as “{value}”</div>
      )}
      <div className="text-xs text-gray-400">
        {assignments} assignment{assignments === 1 ? "" : "s"}
      </div>
    </div>
  );
}
