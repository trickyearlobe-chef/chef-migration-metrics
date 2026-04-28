// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import {
  fetchGitKitchenInstances,
  fetchGitKitchenResults,
  triggerGitKitchenRun,
} from "../api";
import type {
  GitKitchenPlanResult,
  GitKitchenResult,
  GitKitchenInstanceStatus,
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
};

export function GitKitchenSection({ repoName }: { repoName: string }) {
  const { targetChefVersion } = useGlobalFilters();
  const { onEvent } = useWebSocket();
  const [plan, setPlan] = useState<GitKitchenPlanResult | null>(null);
  const [results, setResults] = useState<GitKitchenResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [runningInstance, setRunningInstance] = useState<string | null>(null);
  const [runMessage, setRunMessage] = useState<string | null>(null);
  const [expandedInstance, setExpandedInstance] = useState<string | null>(null);

  const refreshResults = useCallback(() => {
    fetchGitKitchenResults(repoName)
      .then((r) => setResults(r ?? []))
      .catch(() => {});
  }, [repoName]);

  useEffect(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      fetchGitKitchenInstances(repoName),
      fetchGitKitchenResults(repoName),
    ])
      .then(([planData, resultsData]) => {
        setPlan(planData);
        setResults(resultsData ?? []);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [repoName]);

  useEffect(() => {
    return onEvent("git_kitchen_run_complete", (data) => {
      const evt = data as { git_repo_name?: string };
      if (evt.git_repo_name === repoName) {
        refreshResults();
        setRunningInstance(null);
      }
    });
  }, [repoName, onEvent, refreshResults]);

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
      // runningInstance stays set — cleared by WebSocket event
    } catch (e: unknown) {
      setRunMessage(`Run failed: ${(e as Error).message}`);
      setRunningInstance(null);
    }
  }

  return (
    <div>
      <h4 className="mb-2 text-sm font-medium text-gray-600">
        Test Kitchen Instances
      </h4>

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
                  isRunning={runningInstance === inst.instance_name}
                />
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function InstanceRow({
  inst,
  result,
  isExpanded,
  onToggleOutput,
  onRun,
  isRunning,
}: {
  inst: GitKitchenPlanResult["instances"][0];
  result?: GitKitchenResult;
  isExpanded: boolean;
  onToggleOutput: () => void;
  onRun: () => void;
  isRunning: boolean;
}) {
  return (
    <>
      <tr className="border-b border-gray-100">
        <td className="px-2 py-1 font-mono">{inst.instance_name}</td>
        <td className="px-2 py-1">{inst.suite_name}</td>
        <td className="px-2 py-1">{inst.platform_name}</td>
        <td className="px-2 py-1">
          <StatusBadge
            variant={statusVariantMap[inst.status]}
            label={inst.status === "excluded" ? "skipped" : inst.status}
            size="sm"
          />
        </td>
        <td className="px-2 py-1 text-gray-500">
          {inst.image_name ?? "—"}
        </td>
        <td className="px-2 py-1">
          <ResultBadge result={result} onClick={result ? onToggleOutput : undefined} />
        </td>
        <td className="px-2 py-1">
          {inst.status === "mapped" && (
            <button
              onClick={onRun}
              disabled={isRunning}
              className="rounded border border-blue-300 bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700 hover:bg-blue-100 disabled:opacity-50"
            >
              {isRunning ? "Running…" : "Run"}
            </button>
          )}
        </td>
      </tr>
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
