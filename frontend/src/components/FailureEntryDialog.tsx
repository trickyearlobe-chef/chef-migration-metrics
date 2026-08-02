// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";
import { GitRepoPicker } from "./GitRepoPicker";
import type {
  FailureRegisterEntry,
  FailureVerdict,
  HolderType,
  RecordFailureVerdictBody,
  ReviseFailureEntryBody,
} from "../types";

type Mode = "record" | "reverse" | "revise";

interface Props {
  mode: Mode;
  entry?: FailureRegisterEntry;
  /** Fixes the subject when recording from a repo's own page — there is
   * nothing to pick, and nothing to mistype. */
  fixedRepo?: { name: string; cookbookName?: string };
  onCancel: () => void;
  /** record and reverse both write a new verdict. */
  onSubmit?: (body: RecordFailureVerdictBody) => Promise<void>;
  /** revise updates what is known and planned, never the verdict. */
  onRevise?: (body: ReviseFailureEntryBody) => Promise<void>;
}

const HOLDER_TYPES: { value: HolderType; label: string }[] = [
  { value: "ticket", label: "A ticket or work item" },
  { value: "owner", label: "An owner" },
  { value: "user", label: "A user" },
];

/**
 * Recording a verdict (journey 4), reversing one, and updating the plan.
 *
 * The split matters: the verdict and its reason are immutable, because a
 * reversal is a new verdict and the old one has to stay readable. Everything
 * else — the diagnosis, the plan, the holder, the target date — arrives after
 * the entry is raised, because a failure is worth recording the moment it is
 * seen and before anybody knows what to do about it.
 */
export function FailureEntryDialog({
  mode,
  entry,
  fixedRepo,
  onCancel,
  onSubmit,
  onRevise,
}: Props) {
  const revising = mode === "revise";

  const [gitRepoName, setGitRepoName] = useState(
    fixedRepo?.name ?? entry?.git_repo_name ?? "",
  );
  const [cookbookName, setCookbookName] = useState(
    fixedRepo?.cookbookName ?? fixedRepo?.name ?? entry?.cookbook_name ?? "",
  );
  // The subject has to be a repo that exists, or the verdict changes nobody's
  // readiness. Already satisfied when the repo came from its own page or from
  // the entry being reversed; otherwise it has to be picked.
  const [repoConfirmed, setRepoConfirmed] = useState(
    Boolean(fixedRepo) || mode === "reverse",
  );
  // A reversal starts on the opposite verdict, which is what it is for.
  const [verdict, setVerdict] = useState<FailureVerdict>(
    mode === "reverse" && entry?.verdict === "broken" ? "not_broken" : "broken",
  );
  const [reason, setReason] = useState("");
  const [evidence, setEvidence] = useState(
    revising ? (entry?.evidence ?? "") : "",
  );
  const [diagnosis, setDiagnosis] = useState(
    revising ? (entry?.diagnosis ?? "") : "",
  );
  const [plan, setPlan] = useState(revising ? (entry?.plan ?? "") : "");
  const [targetDate, setTargetDate] = useState(
    revising ? (entry?.target_date ?? "") : "",
  );
  const [holderType, setHolderType] = useState<HolderType | "">(
    revising ? (entry?.holder_type ?? "") : "",
  );
  const [holderRef, setHolderRef] = useState(
    revising ? (entry?.holder_ref ?? "") : "",
  );

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const title =
    mode === "record"
      ? "Record a failure"
      : mode === "reverse"
        ? `Change the verdict on ${entry?.cookbook_name ?? ""}`
        : `Update ${entry?.cookbook_name ?? ""}`;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    // A holder is a type and a reference together, or neither: half a
    // reference cannot be chased.
    if ((holderType === "") !== (holderRef.trim() === "")) {
      setError(
        "A commitment holder needs both what kind it is and the reference.",
      );
      return;
    }

    setSubmitting(true);
    try {
      if (revising) {
        await onRevise?.({
          diagnosis,
          plan,
          evidence,
          target_date: targetDate,
          holder_type: holderType === "" ? undefined : holderType,
          holder_ref: holderRef,
        });
      } else {
        await onSubmit?.({
          git_repo_name: gitRepoName.trim(),
          cookbook_name: cookbookName.trim(),
          verdict,
          reason,
          evidence: evidence || undefined,
          diagnosis: diagnosis || undefined,
          plan: plan || undefined,
          target_date: targetDate || undefined,
          holder_type: holderType === "" ? undefined : holderType,
          holder_ref: holderRef || undefined,
        });
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to save.");
      setSubmitting(false);
    }
  }

  const canSubmit = revising
    ? true
    : repoConfirmed &&
      gitRepoName.trim() !== "" &&
      cookbookName.trim() !== "" &&
      reason.trim() !== "";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className="max-h-full w-full max-w-2xl overflow-y-auto rounded-lg bg-white p-6 shadow-xl">
        <h3 className="text-lg font-semibold text-gray-800">{title}</h3>

        {mode === "reverse" && entry && (
          <p className="mt-2 rounded-md border border-gray-200 bg-gray-50 px-3 py-2 text-xs text-gray-600">
            This records a new verdict. The current one — “{entry.reason}”,
            raised by {entry.raised_by} — is kept and marked overturned, so the
            disagreement stays on the record.
          </p>
        )}

        <form onSubmit={(e) => void handleSubmit(e)} className="mt-4 space-y-4">
          {!revising && (
            <>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="block space-y-1">
                  <label
                    htmlFor="failure-git-repo"
                    className="block text-sm font-medium text-gray-700"
                  >
                    Git repo
                  </label>
                  {fixedRepo || mode === "reverse" ? (
                    <input
                      id="failure-git-repo"
                      type="text"
                      value={gitRepoName}
                      readOnly
                      className="w-full rounded-md border border-gray-300 bg-gray-50 px-2 py-1.5 text-sm text-gray-700"
                    />
                  ) : (
                    <GitRepoPicker
                      inputId="failure-git-repo"
                      value={gitRepoName}
                      onChange={(repo, rawName) => {
                        setGitRepoName(rawName);
                        setRepoConfirmed(repo !== null);
                        // One cookbook per repo is the assumption throughout,
                        // so the label follows the repo until somebody edits
                        // it. A repo holding several cookbooks is a mono-repo,
                        // deliberately not handled yet.
                        if (repo) setCookbookName((c) => c || repo.name);
                      }}
                    />
                  )}
                  <span className="block text-xs text-gray-500">
                    Where the fix is made. Never a version.
                  </span>
                </div>
                <Field label="Cookbook" hint="What it is called at standup.">
                  <input
                    type="text"
                    value={cookbookName}
                    onChange={(e) => setCookbookName(e.target.value)}
                    readOnly={mode === "reverse"}
                    required
                    className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                  />
                </Field>
              </div>

              {/* A fieldset rather than a Field: wrapping a radio group in a
                  single <label> gives every radio in it the same accessible
                  name, so a screen reader — and anything else reading the
                  page — cannot tell the two verdicts apart. */}
              <fieldset className="space-y-2">
                <legend className="text-sm font-medium text-gray-700">
                  Verdict
                </legend>
                <label className="flex items-start gap-2 text-sm">
                  <input
                    type="radio"
                    name="verdict"
                    value="broken"
                    checked={verdict === "broken"}
                    onChange={() => setVerdict("broken")}
                    className="mt-1"
                  />
                  <span>
                    <strong>This is broken</strong> — it fails on a real
                    converge and no scan or test caught it.
                  </span>
                </label>
                <label className="flex items-start gap-2 text-sm">
                  <input
                    type="radio"
                    name="verdict"
                    value="not_broken"
                    checked={verdict === "not_broken"}
                    onChange={() => setVerdict("not_broken")}
                    className="mt-1"
                  />
                  <span>
                    <strong>This is not broken</strong> — whatever the scans
                    say. It runs in production today.
                  </span>
                </label>
              </fieldset>

              <Field
                label="Reason"
                hint="Required. A verdict with no reason is an opinion, and it will be overturned by the next person who disagrees."
              >
                <textarea
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  rows={3}
                  required
                  className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </Field>
            </>
          )}

          <Field
            label="Evidence"
            hint="Optional but expected — the stacktrace, the run that failed, or the fleet observation that contradicts the scan."
          >
            <textarea
              value={evidence}
              onChange={(e) => setEvidence(e.target.value)}
              rows={4}
              className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 font-mono text-xs"
            />
          </Field>

          <Field label="Diagnosis" hint="Optional — what is actually wrong.">
            <textarea
              value={diagnosis}
              onChange={(e) => setDiagnosis(e.target.value)}
              rows={2}
              className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </Field>

          <Field
            label="Plan"
            hint="Optional — a failure is worth recording before anybody knows what to do about it."
          >
            <textarea
              value={plan}
              onChange={(e) => setPlan(e.target.value)}
              rows={2}
              className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </Field>

          <div className="grid gap-4 sm:grid-cols-3">
            <Field label="Who is on it">
              <select
                value={holderType}
                onChange={(e) =>
                  setHolderType(e.target.value as HolderType | "")
                }
                className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                <option value="">Nobody yet</option>
                {HOLDER_TYPES.map((h) => (
                  <option key={h.value} value={h.value}>
                    {h.label}
                  </option>
                ))}
              </select>
            </Field>
            <Field
              label="Reference"
              hint="CMM holds the reference; it does not read the system behind it."
            >
              <input
                type="text"
                value={holderRef}
                onChange={(e) => setHolderRef(e.target.value)}
                className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
            </Field>
            <Field label="Target date">
              <input
                type="date"
                value={targetDate}
                onChange={(e) => setTargetDate(e.target.value)}
                className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
            </Field>
          </div>

          {error && (
            <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
              {error}
            </div>
          )}

          <div className="flex justify-end gap-2 border-t border-gray-100 pt-4">
            <button
              type="button"
              onClick={onCancel}
              className="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting || !canSubmit}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {submitting
                ? "Saving…"
                : revising
                  ? "Save"
                  : "Record this verdict"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="block space-y-1">
      <span className="block text-sm font-medium text-gray-700">{label}</span>
      {children}
      {hint && <span className="block text-xs text-gray-500">{hint}</span>}
    </label>
  );
}
