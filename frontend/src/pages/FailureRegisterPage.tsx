// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  fetchFailureRegister,
  recordFailureVerdict,
  resolveFailureEntry,
  reviseFailureEntry,
} from "../api";
import { useAuth } from "../context/AuthContext";
import { EmptyState, ErrorAlert, LoadingSpinner } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { FailureEntryDialog } from "../components/FailureEntryDialog";
import type {
  FailureRegisterEntry,
  FailureRegisterResponse,
  FailureRegisterSummary,
  Pagination as PaginationType,
} from "../types";

const PER_PAGE = 25;

const formatDate = (iso?: string) =>
  iso ? new Date(iso).toLocaleDateString() : "";

const HOLDER_LABELS: Record<string, string> = {
  owner: "Owner",
  user: "User",
  ticket: "Tracked as",
};

/**
 * The failure register — journey 6, read together every morning.
 *
 * It has to answer, in one place: which repos are currently broken (labelled
 * by cookbook), why each one is broken, what is being done about it, and
 * whether the list is getting too large. The size and direction matter as much
 * as the contents — a register that is growing is a different message from one
 * that is shrinking.
 *
 * Without this view, recording a failure is data entry nobody reads and people
 * stop doing it within a fortnight.
 */
export function FailureRegisterPage() {
  const { isOperator } = useAuth();

  const [response, setResponse] = useState<FailureRegisterResponse | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState("open");
  // The standup asks what is broken. Entries overruling a wrong automated
  // verdict are the accuracy report rather than the agenda, and are one
  // selection away.
  const [verdict, setVerdict] = useState("broken");
  const [notice, setNotice] = useState<string | null>(null);

  // The dialog does three jobs: raise a new entry, revise the plan on one, and
  // reverse a standing verdict (which records a new one rather than editing).
  const [recording, setRecording] = useState(false);
  const [revising, setRevising] = useState<FailureRegisterEntry | null>(null);
  const [reversing, setReversing] = useState<FailureRegisterEntry | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    return fetchFailureRegister({
      status,
      verdict: verdict || undefined,
      page,
      per_page: PER_PAGE,
    })
      .then(setResponse)
      .catch((e: unknown) =>
        setError(
          e instanceof Error ? e.message : "Failed to read the failure register.",
        ),
      )
      .finally(() => setLoading(false));
  }, [page, status, verdict]);

  useEffect(() => {
    void load();
  }, [load]);

  const entries = response?.data ?? [];
  const summary = response?.summary;
  const pagination: PaginationType | null = response?.pagination ?? null;

  async function handleResolve(entry: FailureRegisterEntry) {
    const note = window.prompt(
      `Resolving “${entry.cookbook_name}”. What was done? (optional)`,
      "",
    );
    if (note === null) return;
    try {
      await resolveFailureEntry(entry.id, note);
      setNotice(`${entry.cookbook_name} has been marked resolved.`);
      void load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to resolve the entry.");
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-gray-800">Failure register</h2>
          <p className="mt-1 text-sm text-gray-500">
            What the team says is actually broken, and what is not broken
            whatever the scans say. A person’s verdict outranks CookStyle and
            Test Kitchen, and readiness follows it.
          </p>
        </div>
        {isOperator && (
          <button
            type="button"
            onClick={() => {
              setNotice(null);
              setRecording(true);
            }}
            className="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm transition-colors hover:bg-blue-700"
          >
            Record a failure
          </button>
        )}
      </div>

      {summary && <RegisterSummary summary={summary} />}

      <div className="flex flex-wrap items-center gap-3 text-sm">
        <label className="flex items-center gap-2">
          <span className="text-gray-600">Show</span>
          <select
            value={status}
            onChange={(e) => {
              setPage(1);
              setStatus(e.target.value);
            }}
            className="rounded-md border border-gray-300 px-2 py-1 text-sm"
          >
            <option value="open">Standing</option>
            <option value="resolved">Resolved</option>
            <option value="superseded">Overturned</option>
            <option value="all">Everything</option>
          </select>
        </label>
        <label className="flex items-center gap-2">
          <span className="text-gray-600">Verdict</span>
          <select
            value={verdict}
            onChange={(e) => {
              setPage(1);
              setVerdict(e.target.value);
            }}
            className="rounded-md border border-gray-300 px-2 py-1 text-sm"
          >
            <option value="broken">Broken — the tools missed it</option>
            <option value="not_broken">
              Not broken — the tools got it wrong
            </option>
            <option value="">Both</option>
          </select>
        </label>
      </div>

      {notice && (
        <div className="rounded-md border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-700">
          {notice}
        </div>
      )}

      {loading ? (
        <LoadingSpinner message="Reading the failure register…" />
      ) : error ? (
        <ErrorAlert message={error} onRetry={() => void load()} />
      ) : entries.length === 0 ? (
        <div className="card">
          <EmptyState
            title={
              status === "open"
                ? "Nothing is on the register"
                : "Nothing to show"
            }
            description={
              status === "open"
                ? "Nobody has recorded a failure the tools missed, or overruled one they got wrong."
                : "No entries match this filter."
            }
          />
        </div>
      ) : (
        <div className="card p-0">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-gray-200 text-xs font-medium uppercase tracking-wider text-gray-500">
                  <th className="px-3 py-2">Cookbook</th>
                  <th className="px-3 py-2">Why</th>
                  <th className="px-3 py-2">Being done about it</th>
                  <th className="px-3 py-2">Raised</th>
                  {isOperator && <th className="px-3 py-2" />}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {entries.map((entry) => (
                  <EntryRow
                    key={entry.id}
                    entry={entry}
                    canEdit={isOperator}
                    onRevise={() => {
                      setNotice(null);
                      setRevising(entry);
                    }}
                    onReverse={() => {
                      setNotice(null);
                      setReversing(entry);
                    }}
                    onResolve={() => void handleResolve(entry)}
                  />
                ))}
              </tbody>
            </table>
          </div>

          {pagination && (
            <div className="border-t border-gray-100 px-3 py-2">
              <Pagination pagination={pagination} onPageChange={setPage} />
            </div>
          )}
        </div>
      )}

      {recording && (
        <FailureEntryDialog
          mode="record"
          onCancel={() => setRecording(false)}
          onSubmit={async (body) => {
            await recordFailureVerdict(body);
            setRecording(false);
            setNotice(`${body.cookbook_name} has been added to the register.`);
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
            setNotice(
              `A new verdict has been recorded for ${body.cookbook_name}. ` +
                "The previous one stays on the record.",
            );
            void load();
          }}
        />
      )}

      {revising && (
        <FailureEntryDialog
          mode="revise"
          entry={revising}
          onCancel={() => setRevising(null)}
          onRevise={async (body) => {
            await reviseFailureEntry(revising.id, body);
            setRevising(null);
            setNotice(`${revising.cookbook_name} has been updated.`);
            void load();
          }}
        />
      )}
    </div>
  );
}

/**
 * Whether the list is getting too large. Direction of travel is the point:
 * more raised than resolved is a growing register.
 */
function RegisterSummary({ summary }: { summary: FailureRegisterSummary }) {
  const net = summary.raised_in_window - summary.resolved_in_window;
  const direction =
    net > 0 ? "growing" : net < 0 ? "shrinking" : "holding steady";
  const directionClass =
    net > 0
      ? "border-red-200 bg-red-50 text-red-800"
      : net < 0
        ? "border-green-200 bg-green-50 text-green-800"
        : "border-gray-200 bg-gray-50 text-gray-700";

  return (
    <div className="space-y-2">
      {/* One strip rather than four tall tiles: this page is read with a lot
          of entries below it, and the counts are context for the list, not
          the point of the page. */}
      <div className="card flex flex-wrap items-center gap-x-6 gap-y-2 py-3">
        <Stat label="on the register" value={summary.open} />
        <Stat label="the tools missed" value={summary.open_broken} />
        <Stat label="the tools got wrong" value={summary.open_not_broken} />
        <Stat label="nobody on it" value={summary.open_without_holder} />
        {summary.open_overdue > 0 && (
          <Stat label="past target" value={summary.open_overdue} alarming />
        )}
      </div>

      <div
        className={`rounded-md border px-3 py-2 text-sm ${directionClass}`}
        data-testid="register-direction"
      >
        Over the last {summary.window_days} days the register is{" "}
        <strong>{direction}</strong> — {summary.raised_in_window} raised,{" "}
        {summary.resolved_in_window} resolved.
        {summary.open_overdue > 0 && (
          <>
            {" "}
            {summary.open_overdue} {summary.open_overdue === 1 ? "is" : "are"}{" "}
            past the target date.
          </>
        )}
      </div>
    </div>
  );
}

function Stat({
  label,
  value,
  alarming,
}: {
  label: string;
  value: number;
  alarming?: boolean;
}) {
  return (
    <div className="flex items-baseline gap-1.5">
      <span
        className={`text-xl font-bold ${alarming ? "text-red-700" : "text-gray-800"}`}
      >
        {value}
      </span>
      <span className="text-xs text-gray-500">{label}</span>
    </div>
  );
}

function EntryRow({
  entry,
  canEdit,
  onRevise,
  onReverse,
  onResolve,
}: {
  entry: FailureRegisterEntry;
  canEdit: boolean;
  onRevise: () => void;
  onReverse: () => void;
  onResolve: () => void;
}) {
  const broken = entry.verdict === "broken";
  const open = entry.status === "open";
  const overdue =
    open && !!entry.target_date && new Date(entry.target_date) < new Date();

  return (
    <tr className="align-top hover:bg-gray-50">
      <td className="px-3 py-2">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="font-medium text-gray-800">
            {entry.cookbook_name}
          </span>
          <span
            className={`rounded-full px-1.5 py-0.5 text-[10px] font-medium ${
              broken ? "bg-red-100 text-red-800" : "bg-green-100 text-green-800"
            }`}
          >
            {broken ? "Broken" : "Not broken"}
          </span>
          {!open && (
            <span className="rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-600">
              {entry.status === "resolved" ? "Resolved" : "Overturned"}
            </span>
          )}
        </div>
        {/* The subject: the repo where the fix is made, or the cookbook
            itself where no repo has been collected. */}
        <Link
          to={
            entry.subject_type === "cookbook"
              ? `/cookbooks?search=${encodeURIComponent(entry.subject_name)}`
              : `/git-repos/${encodeURIComponent(entry.subject_name)}`
          }
          className="text-xs text-blue-600 hover:underline"
        >
          {entry.subject_name}
        </Link>
        {entry.subject_type === "cookbook" && (
          <span className="ml-1 text-[10px] text-gray-400">no repo</span>
        )}
      </td>

      {/* Why, at a glance rather than a click away. */}
      <td className="max-w-md px-3 py-2 text-gray-700">
        <div className="break-words">{entry.reason}</div>
        {entry.diagnosis && (
          <div className="mt-0.5 break-words text-xs text-gray-500">
            {entry.diagnosis}
          </div>
        )}
      </td>

      {/* What is being done about it. */}
      <td className="max-w-md px-3 py-2 text-xs text-gray-600">
        {entry.plan ? (
          <div className="break-words">{entry.plan}</div>
        ) : (
          open && <div className="text-amber-700">No plan yet.</div>
        )}
        <div className="mt-0.5 flex flex-wrap gap-x-3">
          {entry.holder_ref ? (
            <span>
              <span className="text-gray-500">
                {HOLDER_LABELS[entry.holder_type ?? ""] ?? "Held by"}:{" "}
              </span>
              {entry.holder_ref}
            </span>
          ) : (
            open && <span className="text-amber-700">Nobody on it.</span>
          )}
          {entry.target_date && (
            <span className={overdue ? "font-medium text-red-700" : ""}>
              {formatDate(entry.target_date)}
              {overdue && " — overdue"}
            </span>
          )}
        </div>
      </td>

      <td className="whitespace-nowrap px-3 py-2 text-xs text-gray-400">
        {entry.raised_by}
        <div>{formatDate(entry.raised_at)}</div>
        {entry.resolved_at && (
          <div title={entry.resolution_note}>
            resolved {formatDate(entry.resolved_at)}
          </div>
        )}
      </td>

      {canEdit && (
        <td className="whitespace-nowrap px-3 py-2 text-right">
          {open && (
            <div className="flex justify-end gap-1">
              <button
                type="button"
                onClick={onRevise}
                className="rounded-md border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50"
              >
                Update plan
              </button>
              <button
                type="button"
                onClick={onReverse}
                className="rounded-md border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50"
              >
                Change verdict
              </button>
              <button
                type="button"
                onClick={onResolve}
                className="rounded-md bg-blue-600 px-2 py-1 text-xs font-medium text-white hover:bg-blue-700"
              >
                Resolve
              </button>
            </div>
          )}
        </td>
      )}
    </tr>
  );
}
