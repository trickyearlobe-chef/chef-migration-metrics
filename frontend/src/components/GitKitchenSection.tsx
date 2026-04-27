// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect } from "react";
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

const statusVariantMap: Record<GitKitchenInstanceStatus, "compatible" | "warning" | "untested" | "incompatible"> = {
  mapped: "compatible",
  unmapped: "warning",
  skipped: "untested",
  excluded: "incompatible",
};

export function GitKitchenSection({ repoName }: { repoName: string }) {
  const [plan, setPlan] = useState<GitKitchenPlanResult | null>(null);
  const [results, setResults] = useState<GitKitchenResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [runningInstance, setRunningInstance] = useState<string | null>(null);
  const [runMessage, setRunMessage] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      fetchGitKitchenInstances(repoName),
      fetchGitKitchenResults(repoName),
    ])
      .then(([planData, resultsData]) => {
        setPlan(planData);
        setResults(resultsData);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [repoName]);

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
        target_chef_version: "",
      });
      setRunMessage(resp.message);
    } catch (e: unknown) {
      setRunMessage(`Run failed: ${(e as Error).message}`);
    } finally {
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
              return (
                <tr
                  key={inst.instance_name}
                  className="border-b border-gray-100"
                >
                  <td className="px-2 py-1 font-mono">{inst.instance_name}</td>
                  <td className="px-2 py-1">{inst.suite_name}</td>
                  <td className="px-2 py-1">{inst.platform_name}</td>
                  <td className="px-2 py-1">
                    <StatusBadge
                      variant={statusVariantMap[inst.status]}
                      label={inst.status}
                      size="sm"
                    />
                  </td>
                  <td className="px-2 py-1 text-gray-500">
                    {inst.image_name ?? "—"}
                  </td>
                  <td className="px-2 py-1">
                    <ResultBadge result={result} />
                  </td>
                  <td className="px-2 py-1">
                    {inst.status === "mapped" && (
                      <button
                        onClick={() => handleRun(inst.instance_name)}
                        disabled={runningInstance === inst.instance_name}
                        className="rounded border border-blue-300 bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700 hover:bg-blue-100 disabled:opacity-50"
                      >
                        {runningInstance === inst.instance_name
                          ? "Running…"
                          : "Run"}
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ResultBadge({ result }: { result?: GitKitchenResult }) {
  if (!result) return <span className="text-gray-400">—</span>;
  if (result.passed === true) {
    return (
      <span className="inline-flex items-center rounded-full bg-green-100 px-1.5 py-0.5 text-[10px] font-medium text-green-800">
        ✓ Passed
      </span>
    );
  }
  if (result.passed === false) {
    return (
      <span className="inline-flex items-center rounded-full bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-800">
        ✗ Failed
      </span>
    );
  }
  return (
    <span className="inline-flex items-center rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-600">
      Running…
    </span>
  );
}
