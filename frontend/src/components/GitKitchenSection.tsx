// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import {
  fetchGitKitchenInstances,
  fetchGitKitchenResults,
  triggerGitKitchenRun,
  triggerGitKitchenRunAll,
  fetchKitchenExclusions,
  createKitchenExclusion,
  deleteKitchenExclusion,
} from "../api";
import type {
  GitKitchenPlanResult,
  GitKitchenResult,
  GitKitchenInstanceStatus,
  KitchenInstanceExclusion,
} from "../types";
import { LoadingSpinner, ErrorAlert } from "./Feedback";
import { StatusBadge } from "./StatusBadge";
import { useGlobalFilters } from "../context/GlobalFilterContext";
import { useWebSocket } from "../hooks/useWebSocket";

const statusVariantMap: Record<GitKitchenInstanceStatus, "compatible" | "warning" | "untested" | "incompatible"> = {
  mapped: "compatible",
  unmapped: "warning",
  skipped: "untested",
  excluded: "incompatible",
  user_excluded: "incompatible",
};

export function GitKitchenSection({ repoName }: { repoName: string }) {
  const { targetChefVersion } = useGlobalFilters();
  const { onEvent } = useWebSocket();
  const [plan, setPlan] = useState<GitKitchenPlanResult | null>(null);
  const [results, setResults] = useState<GitKitchenResult[]>([]);
  const [exclusions, setExclusions] = useState<KitchenInstanceExclusion[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [runningInstance, setRunningInstance] = useState<string | null>(null);
  const [runningAll, setRunningAll] = useState(false);
  const [runMessage, setRunMessage] = useState<string | null>(null);
  const [expandedInstance, setExpandedInstance] = useState<string | null>(null);
  const [excludeTarget, setExcludeTarget] = useState<{ suite: string; platform: string } | null>(null);
  const [excludeReason, setExcludeReason] = useState("");
  const [excludeSubmitting, setExcludeSubmitting] = useState(false);

  const refreshData = useCallback(() => {
    Promise.all([
      fetchGitKitchenInstances(repoName),
      fetchGitKitchenResults(repoName),
      fetchKitchenExclusions(repoName),
    ])
      .then(([planData, resultsData, exclData]) => {
        setPlan(planData);
        setResults(resultsData ?? []);
        setExclusions(exclData ?? []);
      })
      .catch(() => {});
  }, [repoName]);

  useEffect(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      fetchGitKitchenInstances(repoName),
      fetchGitKitchenResults(repoName),
      fetchKitchenExclusions(repoName),
    ])
      .then(([planData, resultsData, exclData]) => {
        setPlan(planData);
        setResults(resultsData ?? []);
        setExclusions(exclData ?? []);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [repoName]);

  useEffect(() => {
    return onEvent("git_kitchen_run_complete", (data) => {
      const evt = data as { git_repo_name?: string };
      if (evt.git_repo_name === repoName) {
        refreshData();
        setRunningInstance(null);
        setRunningAll(false);
      }
    });
  }, [repoName, onEvent, refreshData]);

  if (loading) return <LoadingSpinner message="Loading kitchen instances…" />;
  if (error) return <ErrorAlert message={error} />;
  if (!plan || plan.instances.length === 0) return null;

  function latestResult(instanceName: string): GitKitchenResult | undefined {
    return results
      .filter((r) => r.instance_name === instanceName)
      .sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
  }

  async function handleRun(instanceName: string) {
    setRunningInstance(instanceName);
    setRunMessage(null);
    try {
      const resp = await triggerGitKitchenRun({
        git_repo_name: repoName,
        instance_name: instanceName,
        target_chef_version: targetChefVersion,
      });
      setRunMessage(resp.message);
    } catch (e: unknown) {
      setRunMessage(`Run failed: ${(e as Error).message}`);
      setRunningInstance(null);
    }
  }

  async function handleRunAll() {
    setRunningAll(true);
    setRunMessage(null);
    try {
      const resp = await triggerGitKitchenRunAll({
        git_repo_name: repoName,
        target_chef_version: targetChefVersion,
      });
      setRunMessage(`${resp.message} (${resp.instance_count} instances)`);
    } catch (e: unknown) {
      setRunMessage(`Run all failed: ${(e as Error).message}`);
      setRunningAll(false);
    }
  }

  async function handleExcludeSubmit() {
    if (!excludeTarget || !plan) return;
    setExcludeSubmitting(true);
    try {
      await createKitchenExclusion({
        git_repo_name: plan.git_repo_name,
        git_repo_url: plan.git_repo_url,
        suite_name: excludeTarget.suite,
        platform_name: excludeTarget.platform,
        reason: excludeReason.trim(),
      });
      setExcludeTarget(null);
      setExcludeReason("");
      refreshData();
    } catch (e: unknown) {
      setRunMessage(`Exclude failed: ${(e as Error).message}`);
    } finally {
      setExcludeSubmitting(false);
    }
  }

  async function handleRemoveExclusion(id: string) {
    try {
      await deleteKitchenExclusion(id);
      refreshData();
    } catch (e: unknown) {
      setRunMessage(`Remove exclusion failed: ${(e as Error).message}`);
    }
  }

  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <h4 className="text-sm font-medium text-gray-600">
          Test Kitchen Instances
        </h4>
        <button
          onClick={handleRunAll}
          disabled={runningAll || !!runningInstance}
          className="inline-flex items-center gap-1.5 rounded-md border border-green-300 bg-white px-3 py-1 text-xs font-medium text-green-700 shadow-sm hover:bg-green-50 disabled:cursor-not-allowed disabled:opacity-50"
          title="Run all mapped (non-excluded) instances for this repo"
        >
          {runningAll ? "Running…" : "Run All Suites"}
        </button>
      </div>

      {runMessage && (
        <div className="mb-2 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-sm text-blue-800">
          {runMessage}
        </div>
      )}

      <div className="overflow-x-auto">
        <table className="min-w-full text-xs">
          <thead>
            <tr className="border-b border-gray-200 text-left text-gray-500">
              <th className="px-2 py-1 font-medium">Instance Name</th>
              <th className="px-2 py-1 font-medium">Suite</th>
              <th className="px-2 py-1 font-medium">Platform</th>
              <th className="px-2 py-1 font-medium">Status</th>
              <th className="px-2 py-1 font-medium">Image</th>
              <th className="px-2 py-1 font-medium">Result</th>
              <th className="px-2 py-1 font-medium">Action</th>
            </tr>
          </thead>
          <tbody>
            {plan.instances.map((inst) => {
              const result = latestResult(inst.instance_name);
              const isExpanded = expandedInstance === inst.instance_name;
              const isTarget = excludeTarget?.suite === inst.suite_name && excludeTarget?.platform === inst.platform_name;
              return (
                <InstanceRow
                  key={inst.instance_name}
                  inst={inst}
                  result={result}
                  isExpanded={isExpanded}
                  onToggleOutput={() =>
                    setExpandedInstance(isExpanded ? null : inst.instance_name)
                  }
                  onRun={() => handleRun(inst.instance_name)}
                  onExclude={() => setExcludeTarget({ suite: inst.suite_name, platform: inst.platform_name })}
                  isRunning={runningInstance === inst.instance_name}
                  isExcludeTarget={isTarget}
                  excludeReason={isTarget ? excludeReason : ""}
                  onExcludeReasonChange={setExcludeReason}
                  onExcludeSubmit={handleExcludeSubmit}
                  onExcludeCancel={() => { setExcludeTarget(null); setExcludeReason(""); }}
                  excludeSubmitting={excludeSubmitting}
                />
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Current exclusions */}
      {exclusions.length > 0 && (
        <div className="mt-4">
          <h5 className="text-xs font-medium text-gray-500 mb-1">
            Manual Exclusions ({exclusions.length})
          </h5>
          <div className="space-y-1">
            {exclusions.map((ex) => (
              <div key={ex.id} className="flex items-start gap-2 rounded border border-amber-200 bg-amber-50 px-2 py-1 text-xs">
                <span className="font-mono text-amber-800">{ex.suite_name}/{ex.platform_name}</span>
                <span className="text-gray-600 flex-1">{ex.reason}</span>
                <span className="text-gray-400 whitespace-nowrap">by {ex.excluded_by}</span>
                <button
                  onClick={() => handleRemoveExclusion(ex.id)}
                  className="text-red-600 hover:text-red-800 font-medium"
                  title="Remove exclusion"
                >
                  ✕
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function InstanceRow({
  inst,
  result,
  isExpanded,
  onToggleOutput,
  onRun,
  onExclude,
  isRunning,
  isExcludeTarget,
  excludeReason,
  onExcludeReasonChange,
  onExcludeSubmit,
  onExcludeCancel,
  excludeSubmitting,
}: {
  inst: GitKitchenPlanResult["instances"][0];
  result?: GitKitchenResult;
  isExpanded: boolean;
  onToggleOutput: () => void;
  onRun: () => void;
  onExclude: () => void;
  isRunning: boolean;
  isExcludeTarget: boolean;
  excludeReason: string;
  onExcludeReasonChange: (v: string) => void;
  onExcludeSubmit: () => void;
  onExcludeCancel: () => void;
  excludeSubmitting: boolean;
}) {
  const statusLabel = inst.status === "user_excluded" ? "excluded" : inst.status === "excluded" ? "skipped" : inst.status;

  return (
    <>
      <tr className="border-b border-gray-100">
        <td className="px-2 py-1 font-mono">{inst.instance_name}</td>
        <td className="px-2 py-1">{inst.suite_name}</td>
        <td className="px-2 py-1">{inst.platform_name}</td>
        <td className="px-2 py-1">
          <StatusBadge
            variant={statusVariantMap[inst.status]}
            label={statusLabel}
            size="sm"
          />
          {inst.status === "user_excluded" && (
            <span className="ml-1 text-[10px] text-gray-500" title={inst.status_reason}>
              ({inst.status_reason.length > 40 ? inst.status_reason.slice(0, 40) + "…" : inst.status_reason})
            </span>
          )}
        </td>
        <td className="px-2 py-1 text-gray-500">
          {inst.image_name ?? "—"}
        </td>
        <td className="px-2 py-1">
          <ResultBadge result={result} onClick={result ? onToggleOutput : undefined} />
        </td>
        <td className="px-2 py-1 flex gap-1">
          {inst.status === "mapped" && (
            <button
              onClick={onRun}
              disabled={isRunning}
              className="rounded border border-blue-300 bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700 hover:bg-blue-100 disabled:opacity-50"
            >
              {isRunning ? "Running…" : "Run"}
            </button>
          )}
          {inst.status === "mapped" && result?.passed === false && !isExcludeTarget && (
            <button
              onClick={onExclude}
              className="rounded border border-amber-300 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 hover:bg-amber-100"
              title="Exclude this instance from future runs"
            >
              Exclude
            </button>
          )}
        </td>
      </tr>
      {isExcludeTarget && (
        <tr className="border-b border-amber-200 bg-amber-50">
          <td colSpan={7} className="px-2 py-2">
            <p className="text-xs font-medium text-amber-800 mb-1">
              Exclude <span className="font-mono">{inst.suite_name}/{inst.platform_name}</span> — provide a reason:
            </p>
            <textarea
              value={excludeReason}
              onChange={(e) => onExcludeReasonChange(e.target.value)}
              placeholder="Why is this instance being excluded? (min 10 chars)"
              className="w-full rounded border border-amber-300 bg-white px-2 py-1 text-xs"
              rows={2}
              autoFocus
            />
            <div className="mt-1 flex gap-2">
              <button
                onClick={onExcludeSubmit}
                disabled={excludeReason.trim().length < 10 || excludeSubmitting}
                className="rounded border border-amber-400 bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800 hover:bg-amber-200 disabled:opacity-50"
              >
                {excludeSubmitting ? "Saving…" : "Confirm Exclusion"}
              </button>
              <button
                onClick={onExcludeCancel}
                className="rounded border border-gray-300 px-2 py-0.5 text-xs text-gray-600 hover:bg-gray-100"
              >
                Cancel
              </button>
            </div>
          </td>
        </tr>
      )}
      {isExpanded && result && (
        <tr className="border-b border-gray-200 bg-gray-50">
          <td colSpan={7} className="px-2 py-2">
            <div className="flex items-center gap-2 mb-1 text-[10px] text-gray-500">
              <span>Duration: {result.duration_seconds ?? "—"}s</span>
              <span>·</span>
              <span>Target: {result.target_chef_version}</span>
              <span>·</span>
              <span>Commit: {result.commit_sha?.slice(0, 8)}</span>
              {result.error_message && (
                <>
                  <span>·</span>
                  <span className="text-red-600">Error: {result.error_message}</span>
                </>
              )}
            </div>
            <pre className="max-h-80 overflow-auto rounded bg-gray-900 p-2 text-[10px] leading-tight text-gray-200 whitespace-pre-wrap">
              {result.output || "(no output captured)"}
            </pre>
          </td>
        </tr>
      )}
    </>
  );
}

function ResultBadge({ result, onClick }: { result?: GitKitchenResult; onClick?: () => void }) {
  if (!result) return <span className="text-gray-400">—</span>;

  const clickProps = onClick
    ? { onClick, role: "button" as const, tabIndex: 0, className: "cursor-pointer" }
    : {};

  if (result.passed === true) {
    return (
      <span {...clickProps}>
        <span className="inline-flex items-center rounded-full bg-green-100 px-1.5 py-0.5 text-[10px] font-medium text-green-800">
          ✓ Passed
        </span>
      </span>
    );
  }
  if (result.passed === false) {
    return (
      <span {...clickProps}>
        <span className="inline-flex items-center rounded-full bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-800">
          ✗ Failed
        </span>
      </span>
    );
  }
  return (
    <span className="inline-flex items-center rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-600">
      Running…
    </span>
  );
}
