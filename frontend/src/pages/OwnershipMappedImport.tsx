import { useState, useRef, type ChangeEvent } from "react";
import {
  profileImportSource,
  previewOwnershipImport,
  commitOwnershipImport,
  createImportMapping,
  ApiError,
} from "../api";
import type {
  IntakeFieldMap,
  IntakeFieldMapping,
  IntakeReport,
  IntakeReportRow,
  IntakeSourceProfile,
  IntakeTargetField,
} from "../types";
import { ErrorAlert, LoadingSpinner } from "../components/Feedback";

// ---------------------------------------------------------------------------
// Discovery-driven ownership import.
//
// The source's shape is not known in advance, so this panel walks the
// administrator through: profile the file → map its columns onto CMM's fields →
// preview what would happen → commit. Preview writes nothing, so it can be
// re-run as many times as it takes to get the mapping right.
//
// The fixed-header flow beside this one is untouched and remains the fast lane
// for files already in CMM's own column order.
// ---------------------------------------------------------------------------

const TARGET_FIELDS: {
  key: IntakeTargetField;
  label: string;
  required: boolean;
  hint: string;
}[] = [
  {
    key: "owner",
    label: "Owner",
    required: true,
    hint: "A name, email or username. The original is kept as the display name.",
  },
  {
    key: "entity_type",
    label: "Entity type",
    required: true,
    hint: "The same for every row in the file.",
  },
  {
    key: "entity_key",
    label: "Entity key",
    required: true,
    hint: "The repo, node, cookbook, role or policy being owned.",
  },
  {
    key: "organisation",
    label: "Organisation",
    required: false,
    hint: "Optional. Leave unmapped when the file covers one organisation or none.",
  },
  {
    key: "notes",
    label: "Notes",
    required: false,
    hint: "Optional. Carried onto each assignment.",
  },
  {
    key: "display_name",
    label: "Display name",
    required: false,
    hint: "Optional. Defaults to the owner value before it is turned into a handle.",
  },
];

const ENTITY_TYPES = ["git_repo", "node", "cookbook", "role", "policy"];

const TRANSFORMS = [
  { kind: "", label: "None" },
  { kind: "trim", label: "Trim whitespace" },
  { kind: "lowercase", label: "Lower case" },
  { kind: "strip_domain", label: "Strip email domain (alice@corp → alice)" },
];

const OUTCOME_LABELS: Record<string, string> = {
  would_create: "Would be created",
  duplicate_exists: "Already there",
  owned_by_other: "Also owned by someone else",
  rejected: "Not imported",
};

const REASON_LABELS: Record<string, string> = {
  unknown_owner: "Owner not recognised",
  invalid_entity_type: "Entity type is not valid",
  missing_required_field: "A required value is missing",
  malformed_row: "The row does not have the right number of fields",
  invalid_owner_name: "The owner value cannot become a name",
};

type ColumnChoice = { column: string; transform: string };

export function OwnershipMappedImport() {
  const [file, setFile] = useState<File | null>(null);
  const [delimiter, setDelimiter] = useState("");
  const [profile, setProfile] = useState<IntakeSourceProfile | null>(null);
  const [choices, setChoices] = useState<Partial<Record<IntakeTargetField, ColumnChoice>>>({});
  const [entityType, setEntityType] = useState("git_repo");
  const [createOwners, setCreateOwners] = useState(true);
  const [report, setReport] = useState<IntakeReport | null>(null);
  const [loading, setLoading] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saveName, setSaveName] = useState("");
  const [saved, setSaved] = useState<string | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);

  function reportError(err: unknown, fallback: string) {
    setError(
      err instanceof ApiError || err instanceof Error ? err.message : fallback,
    );
  }

  async function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    const selected = e.target.files?.[0] ?? null;
    setFile(selected);
    setProfile(null);
    setChoices({});
    setReport(null);
    setError(null);
    setSaved(null);
    if (!selected) return;

    setLoading("Reading the file…");
    try {
      const result = await profileImportSource(selected, delimiter || undefined);
      setProfile(result);
      setChoices(guessMapping(result));
    } catch (err: unknown) {
      reportError(err, "Could not read the file.");
    } finally {
      setLoading(null);
    }
  }

  function fieldMap(): IntakeFieldMap {
    const map: IntakeFieldMap = {
      entity_type: { source: { kind: "constant", value: entityType } },
    };
    for (const { key } of TARGET_FIELDS) {
      if (key === "entity_type") continue;
      const choice = choices[key];
      if (!choice?.column) continue;
      const mapping: IntakeFieldMapping = {
        source: { kind: "column", column: choice.column },
      };
      if (choice.transform) {
        mapping.transforms = [{ kind: choice.transform }];
      }
      map[key] = mapping;
    }
    return map;
  }

  const mappingComplete =
    Boolean(choices.owner?.column) && Boolean(choices.entity_key?.column);

  async function run(commit: boolean) {
    if (!file || !mappingComplete) return;

    setLoading(commit ? "Importing…" : "Working out what would happen…");
    setError(null);
    try {
      const opts = {
        file,
        delimiter: delimiter || undefined,
        fieldMap: fieldMap(),
        createOwners,
      };
      setReport(commit ? await commitOwnershipImport(opts) : await previewOwnershipImport(opts));
    } catch (err: unknown) {
      reportError(err, commit ? "The import failed." : "The preview failed.");
    } finally {
      setLoading(null);
    }
  }

  async function handleSaveMapping() {
    if (!saveName.trim()) return;
    setError(null);
    try {
      const mapping = await createImportMapping({
        name: saveName.trim(),
        delimiter: delimiter || ",",
        field_map: fieldMap(),
      });
      setSaved(`Saved as "${mapping.name}". A repeat import can reuse it.`);
      setSaveName("");
    } catch (err: unknown) {
      reportError(err, "Could not save the mapping.");
    }
  }

  function setChoice(field: IntakeTargetField, patch: Partial<ColumnChoice>) {
    setChoices((prev) => ({
      ...prev,
      [field]: { column: "", transform: "", ...prev[field], ...patch },
    }));
    setReport(null);
  }

  return (
    <div className="space-y-6">
      <div className="card">
        <h3 className="card-header">1. Choose a file</h3>
        <p className="mb-3 text-sm text-gray-600">
          Any delimited file. CMM reads its columns and shows you what it found —
          the file does not have to be in CMM's format.
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <input
            ref={fileInputRef}
            type="file"
            accept=".csv,.tsv,.txt"
            onChange={handleFileChange}
            className="block text-sm text-gray-600 file:mr-3 file:rounded-md file:border-0 file:bg-blue-600 file:px-4 file:py-2 file:text-sm file:font-medium file:text-white hover:file:bg-blue-700"
          />
          <label className="flex items-center gap-2 text-sm text-gray-600">
            Delimiter
            <input
              type="text"
              value={delimiter}
              maxLength={1}
              placeholder="auto"
              onChange={(e) => setDelimiter(e.target.value)}
              className="w-20 rounded-md border border-gray-300 px-2 py-1 text-sm"
            />
          </label>
        </div>
        <p className="mt-2 text-xs text-gray-500">
          CMM guesses the delimiter. If it guesses wrong, set it here and choose
          the file again — the guess is never binding.
        </p>
      </div>

      {error && <ErrorAlert message={error} />}
      {loading && <LoadingSpinner message={loading} />}

      {profile && !loading && (
        <>
          <div className="card">
            <h3 className="card-header">
              2. What CMM found — {profile.row_count.toLocaleString()} rows,{" "}
              {profile.columns.length} columns
            </h3>
            {profile.warnings.length > 0 && (
              <ul className="mb-3 space-y-1 rounded-md bg-amber-50 px-4 py-3 text-sm text-amber-800">
                {profile.warnings.map((warning) => (
                  <li key={warning}>{warning}</li>
                ))}
              </ul>
            )}
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <th>Column</th>
                    <th>Filled</th>
                    <th>Distinct values</th>
                    <th>Examples</th>
                  </tr>
                </thead>
                <tbody>
                  {profile.columns.map((column) => (
                    <tr key={column.name}>
                      <td className="font-mono">{column.name}</td>
                      <td>{column.non_empty_pct.toFixed(0)}%</td>
                      <td>{column.distinct_count.toLocaleString()}</td>
                      <td className="whitespace-normal text-gray-500">
                        {column.sample_values.slice(0, 4).join(", ") || "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <div className="card">
            <h3 className="card-header">3. Map the columns</h3>
            <div className="space-y-4">
              <label className="flex flex-wrap items-center gap-3 text-sm">
                <span className="w-32 font-medium text-gray-700">
                  Entity type <span className="text-red-500">*</span>
                </span>
                <select
                  value={entityType}
                  onChange={(e) => {
                    setEntityType(e.target.value);
                    setReport(null);
                  }}
                  className="rounded-md border border-gray-300 px-2 py-1 text-sm"
                >
                  {ENTITY_TYPES.map((type) => (
                    <option key={type} value={type}>
                      {type}
                    </option>
                  ))}
                </select>
                <span className="text-xs text-gray-500">
                  The same for every row in the file.
                </span>
              </label>

              {TARGET_FIELDS.filter((f) => f.key !== "entity_type").map((field) => (
                <label
                  key={field.key}
                  className="flex flex-wrap items-center gap-3 text-sm"
                >
                  <span className="w-32 font-medium text-gray-700">
                    {field.label}
                    {field.required && <span className="text-red-500"> *</span>}
                  </span>
                  <select
                    value={choices[field.key]?.column ?? ""}
                    onChange={(e) => setChoice(field.key, { column: e.target.value })}
                    className="rounded-md border border-gray-300 px-2 py-1 text-sm"
                  >
                    <option value="">Not mapped</option>
                    {profile.columns.map((column) => (
                      <option key={column.name} value={column.name}>
                        {column.name}
                      </option>
                    ))}
                  </select>
                  <select
                    value={choices[field.key]?.transform ?? ""}
                    onChange={(e) => setChoice(field.key, { transform: e.target.value })}
                    disabled={!choices[field.key]?.column}
                    className="rounded-md border border-gray-300 px-2 py-1 text-sm disabled:opacity-50"
                  >
                    {TRANSFORMS.map((t) => (
                      <option key={t.kind} value={t.kind}>
                        {t.label}
                      </option>
                    ))}
                  </select>
                  <span className="text-xs text-gray-500">{field.hint}</span>
                </label>
              ))}

              <label className="flex items-center gap-2 border-t border-gray-100 pt-4 text-sm text-gray-700">
                <input
                  type="checkbox"
                  checked={createOwners}
                  onChange={(e) => {
                    setCreateOwners(e.target.checked);
                    setReport(null);
                  }}
                  className="h-4 w-4 text-blue-600"
                />
                Create owners that CMM does not already know about
                <span className="text-xs text-gray-500">
                  (people who look close to an existing owner are never created —
                  they are listed for you to confirm)
                </span>
              </label>
            </div>

            <div className="mt-4 flex flex-wrap items-center gap-3">
              <button
                type="button"
                onClick={() => run(false)}
                disabled={!mappingComplete}
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                Preview — nothing is saved
              </button>
              <input
                type="text"
                value={saveName}
                onChange={(e) => setSaveName(e.target.value)}
                placeholder="Name this mapping"
                className="rounded-md border border-gray-300 px-2 py-1 text-sm"
              />
              <button
                type="button"
                onClick={handleSaveMapping}
                disabled={!mappingComplete || !saveName.trim()}
                className="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
              >
                Save mapping
              </button>
              {saved && <span className="text-sm text-green-700">{saved}</span>}
            </div>
          </div>
        </>
      )}

      {report && !loading && <IntakeReportView report={report} onCommit={() => run(true)} />}
    </div>
  );
}

// ---------------------------------------------------------------------------
// The match report
// ---------------------------------------------------------------------------

function IntakeReportView({
  report,
  onCommit,
}: {
  report: IntakeReport;
  onCommit: () => void;
}) {
  const rejected = report.rows.filter((row) => row.outcome === "rejected");

  return (
    <div className="space-y-6">
      <div className="card">
        <h3 className="card-header">
          {report.committed ? "Imported" : "4. What would happen"}
        </h3>
        <div className="flex flex-wrap gap-8">
          {Object.entries(OUTCOME_LABELS).map(([outcome, label]) => (
            <div key={outcome} className="flex flex-col">
              <span className="text-sm font-medium text-gray-500">{label}</span>
              <span
                className={
                  "mt-1 text-2xl font-bold " +
                  (outcome === "rejected" ? "text-red-600" : "text-gray-800")
                }
              >
                {report.counts[outcome] ?? 0}
              </span>
            </div>
          ))}
        </div>
        {report.alias_conflict_count > 0 && (
          <p className="mt-4 text-sm text-gray-600">
            {report.alias_conflict_count} row
            {report.alias_conflict_count === 1 ? " has" : "s have"} an owner name
            already recorded against somebody else. Those assignments are still
            made — only the name is not recorded a second time.
          </p>
        )}
        {!report.committed && (
          <div className="mt-4">
            <button
              type="button"
              onClick={onCommit}
              className="rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-green-700"
            >
              Import these {report.counts.would_create ?? 0} assignments
            </button>
          </div>
        )}
        {report.committed && (
          <p className="mt-4 text-sm text-green-700">
            {report.created} assignment{report.created === 1 ? "" : "s"} imported.
          </p>
        )}
      </div>

      {(report.new_owners?.length ?? 0) > 0 && (
        <div className="card">
          <h3 className="card-header">
            People new to CMM ({report.new_owners.length})
          </h3>
          <p className="mb-3 text-sm text-gray-600">
            Worth a scan before you import: someone here may be a person CMM
            already knows under a different name. Nothing is guessed — a nickname
            shares too little with the name on the account for any matching to
            spot it, but you will. If one slips through, it can be merged into
            the right owner later; the import does not wait for this.
          </p>
          <div className="table-container">
            <table className="table">
              <thead>
                <tr>
                  <th>Name in the file</th>
                  <th>Will be added as</th>
                  <th>Rows</th>
                  <th>Might already be</th>
                </tr>
              </thead>
              <tbody>
                {report.new_owners.map((owner) => (
                  <tr key={owner.name}>
                    <td>{owner.display_name || owner.source_value}</td>
                    <td className="font-mono text-gray-500">{owner.name}</td>
                    <td>{owner.row_count}</td>
                    <td className="whitespace-normal text-gray-500">
                      {owner.suggestions?.length
                        ? owner.suggestions.map((s) => s.owner_name).join(", ")
                        : "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {(report.unmatched_owners?.length ?? 0) > 0 && (
        <div className="card">
          <h3 className="card-header">Owners CMM could not place</h3>
          <p className="mb-3 text-sm text-gray-600">
            Nothing is guessed. Create the owner, or add the name below as one of
            their aliases, then run the import again.
          </p>
          <div className="table-container">
            <table className="table">
              <thead>
                <tr>
                  <th>Value in the file</th>
                  <th>Rows</th>
                  <th>Did you mean</th>
                </tr>
              </thead>
              <tbody>
                {report.unmatched_owners.map((unmatched) => (
                  <tr key={unmatched.value}>
                    <td className="font-mono">{unmatched.value}</td>
                    <td>{unmatched.count}</td>
                    <td className="whitespace-normal text-gray-500">
                      {suggestionsFor(report.rows, unmatched.value) || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {rejected.length > 0 && (
        <div className="card">
          <h3 className="card-header">Rows not imported ({rejected.length})</h3>
          <div className="table-container">
            <table className="table">
              <thead>
                <tr>
                  <th>Row</th>
                  <th>Owner in the file</th>
                  <th>Entity</th>
                  <th>Why</th>
                </tr>
              </thead>
              <tbody>
                {rejected.slice(0, 200).map((row) => (
                  <tr key={row.source_row}>
                    <td className="font-mono">{row.source_row}</td>
                    <td className="font-mono">{row.owner_raw || "—"}</td>
                    <td className="font-mono">{row.entity_key || "—"}</td>
                    <td className="whitespace-normal text-red-700">
                      {REASON_LABELS[row.rejected_reason ?? ""] ??
                        row.rejected_reason ??
                        "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {rejected.length > 200 && (
            <p className="mt-2 text-xs text-gray-500">
              Showing the first 200 of {rejected.length}.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function suggestionsFor(rows: IntakeReportRow[], value: string): string {
  const row = rows.find((r) => r.owner_raw === value && r.owner_suggestions?.length);
  if (!row?.owner_suggestions) return "";
  return row.owner_suggestions.map((s) => s.owner_name).join(", ");
}

// ---------------------------------------------------------------------------
// Guessing a starting mapping
//
// A guess the administrator corrects beats an empty form. It is only ever a
// starting point — every field stays editable, and nothing is applied until
// they preview it.
// ---------------------------------------------------------------------------

function guessMapping(
  profile: IntakeSourceProfile,
): Partial<Record<IntakeTargetField, ColumnChoice>> {
  const guesses: Partial<Record<IntakeTargetField, ColumnChoice>> = {};

  const find = (...needles: string[]) =>
    profile.columns.find((column) => {
      const name = column.name.toLowerCase();
      return needles.some((needle) => name.includes(needle));
    })?.name;

  const owner = find("owner", "email", "contact", "maintainer", "assignee", "user");
  if (owner) {
    guesses.owner = { column: owner, transform: "trim" };
  }

  const key = find("repo", "cookbook", "node", "entity", "name");
  if (key) {
    guesses.entity_key = { column: key, transform: "trim" };
  }

  const org = find("org", "tenant", "business");
  if (org && org !== owner && org !== key) {
    guesses.organisation = { column: org, transform: "trim" };
  }

  return guesses;
}
