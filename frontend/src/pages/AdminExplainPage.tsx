// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";
import {
  fetchExplainCatalog,
  runExplain,
  type ExplainCatalogEntry,
  type ExplainResponse,
} from "../api";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";

// PREFILL_KEY carries a query from the Performance tab's per-row "Explain"
// action into the free-text box here (pg_stat_statements text is normalised, so
// it can only be pre-filled for the operator to run — not one-click ANALYZEd).
export const PREFILL_KEY = "cmm-explain-prefill";

const cardCls = "rounded-lg border border-gray-200 bg-white p-4 shadow-sm";
const hdrCls = "text-sm font-semibold text-gray-700";
const btnCls =
  "rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50";

function planReport(r: ExplainResponse): string {
  const run = (label: string, run: ExplainResponse["run1"]) =>
    `\n--- ${label} (${run.duration_ms.toFixed(1)} ms${run.truncated ? ", truncated" : ""}) ---\n${run.plan_text}`;
  return [
    `# EXPLAIN: ${r.label}`,
    `Captured:     ${r.captured_at}`,
    `App version:  ${r.app_version}`,
    `Params:       ${r.param_summary}`,
    `Analyze:      ${r.analyze}    statement_timeout: ${r.statement_timeout_ms} ms`,
    r.sql ? `\nSQL:\n${r.sql}` : "",
    run("Run 1", r.run1),
    r.run2 ? run("Run 2 (warm)", r.run2) : "",
    "",
  ].join("\n");
}

export function AdminExplainPage() {
  const [entries, setEntries] = useState<ExplainCatalogEntry[]>([]);
  const [catalogKey, setCatalogKey] = useState("");
  const [sql, setSql] = useState("");
  const [analyze, setAnalyze] = useState(true);
  const [runTwice, setRunTwice] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ExplainResponse | null>(null);

  useEffect(() => {
    fetchExplainCatalog()
      .then((r) => {
        setEntries(r.entries);
        if (r.entries.length > 0) setCatalogKey((k) => k || r.entries[0].key);
      })
      .catch(() => setError("Failed to load the explain catalog."));

    const prefill = sessionStorage.getItem(PREFILL_KEY);
    if (prefill) {
      setSql(prefill);
      sessionStorage.removeItem(PREFILL_KEY);
    }
  }, []);

  const run = async (req: { catalog_key?: string; sql?: string }) => {
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      setResult(await runExplain({ ...req, analyze, run_twice: runTwice }));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const download = () => {
    if (!result) return;
    const blob = new Blob([planReport(result)], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `explain-${result.label.toLowerCase().replace(/[^a-z0-9]+/g, "-")}.txt`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="space-y-4">
      <p className="text-sm text-gray-500">
        Capture query plans for the hot queries the Performance tab surfaces. Runs are read-only
        and time-limited. <strong>Analyze</strong> executes the query to get real timings — use it
        with care on genuinely slow queries.
      </p>

      <div className="grid gap-4 md:grid-cols-2">
        {/* Canned catalog */}
        <section className={cardCls}>
          <h2 className={hdrCls + " mb-2"}>Canned explains</h2>
          <select
            aria-label="Canned explain"
            className="mb-2 w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
            value={catalogKey}
            onChange={(e) => setCatalogKey(e.target.value)}
          >
            {entries.map((e) => (
              <option key={e.key} value={e.key}>
                {e.label}
              </option>
            ))}
          </select>
          <p className="mb-2 min-h-[2.5rem] text-xs text-gray-500">
            {entries.find((e) => e.key === catalogKey)?.description}
          </p>
          <button
            className={btnCls}
            disabled={loading || !catalogKey}
            onClick={() => run({ catalog_key: catalogKey })}
          >
            Run canned explain
          </button>
        </section>

        {/* Free-text */}
        <section className={cardCls}>
          <h2 className={hdrCls + " mb-2"}>Custom query</h2>
          <textarea
            aria-label="Custom SQL"
            className="mb-2 h-24 w-full rounded border border-gray-300 px-2 py-1.5 font-mono text-xs"
            placeholder="SELECT / UPDATE / DELETE … (single data statement; writes are shown plan-only)"
            value={sql}
            onChange={(e) => setSql(e.target.value)}
          />
          <button
            className={btnCls}
            disabled={loading || sql.trim() === ""}
            onClick={() => run({ sql })}
          >
            Run custom explain
          </button>
        </section>
      </div>

      <div className="flex flex-wrap items-center gap-4 text-sm">
        <label className="flex items-center gap-1.5">
          <input type="checkbox" checked={analyze} onChange={(e) => setAnalyze(e.target.checked)} />
          Analyze (executes the query)
        </label>
        <label className="flex items-center gap-1.5">
          <input type="checkbox" checked={runTwice} onChange={(e) => setRunTwice(e.target.checked)} />
          Run twice (show buffer-cache warmth)
        </label>
      </div>

      {loading && <LoadingSpinner />}
      {error && <ErrorAlert message={error} />}

      {result && (
        <section className={cardCls}>
          <div className="mb-2 flex items-center justify-between">
            <h2 className={hdrCls}>
              {result.label}{" "}
              <span className="font-normal text-gray-400">— {result.param_summary}</span>
            </h2>
            <button
              className="rounded border border-gray-300 px-2 py-1 text-xs hover:bg-gray-50"
              onClick={download}
            >
              Download .txt
            </button>
          </div>
          {result.note && (
            <p className="mb-2 rounded bg-amber-50 px-3 py-2 text-xs text-amber-800">{result.note}</p>
          )}
          <PlanBlock title={`Run 1 (${result.run1.duration_ms.toFixed(1)} ms)`} text={result.run1.plan_text} />
          {result.run2 && (
            <PlanBlock
              title={`Run 2 / warm (${result.run2.duration_ms.toFixed(1)} ms)`}
              text={result.run2.plan_text}
            />
          )}
        </section>
      )}
    </div>
  );
}

function PlanBlock({ title, text }: { title: string; text: string }) {
  return (
    <div className="mb-3">
      <p className="mb-1 text-xs font-medium text-gray-600">{title}</p>
      <pre className="overflow-x-auto rounded bg-gray-900 p-3 font-mono text-xs text-gray-100">
        {text}
      </pre>
    </div>
  );
}
