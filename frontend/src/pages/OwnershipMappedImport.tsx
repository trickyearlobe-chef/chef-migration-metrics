import { useState, useRef, useEffect, type ChangeEvent } from "react";
import {
  profileImportSource,
  profileImportDatabase,
  listImportDatabaseTables,
  previewOwnershipImport,
  commitOwnershipImport,
  createImportMapping,
  fetchImportMappings,
  ApiError,
} from "../api";
import { CronDescription } from "../components/CronDescription";
import { OwnershipConnectionPanel } from "./OwnershipConnectionPanel";
import type { IntakeDatabaseSource, IntakeDatabaseTable } from "../api";
import type {
  IntakeFieldMap,
  IntakeFieldMapping,
  IntakeMapping,
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
  // Import one kind of row from a consolidated export, rather than splitting
  // the file outside CMM first.
  const [filterColumn, setFilterColumn] = useState("");
  const [filterValue, setFilterValue] = useState("");
  const [report, setReport] = useState<IntakeReport | null>(null);
  const [loading, setLoading] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saveName, setSaveName] = useState("");
  const [saved, setSaved] = useState<string | null>(null);
  // Saved mappings, so a second file with the same columns is not re-mapped by
  // hand. Re-mapping is not just tedious: a mapping done twice can differ, and
  // nothing would tell you which import used which.
  const [mappings, setMappings] = useState<IntakeMapping[]>([]);
  const [loadedMapping, setLoadedMapping] = useState<string | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    // Best-effort: not being able to list saved mappings must not stop
    // somebody importing by hand.
    fetchImportMappings()
      .then((r) => setMappings(r.data ?? []))
      .catch(() => setMappings([]));
  }, []);

  // Turn a stored mapping document back into the form's state. The reverse of
  // fieldMap(): a constant entity_type is the entity selector, a column source
  // is a column choice, and the first transform is the one the form can show.
  function applyMapping(m: IntakeMapping) {
    const next: Partial<Record<IntakeTargetField, ColumnChoice>> = {};
    const missing: string[] = [];
    const known = new Set((profile?.columns ?? []).map((c) => c.name));

    for (const [field, mapping] of Object.entries(m.field_map ?? {})) {
      const src = mapping.source;
      if (field === "entity_type" && src.kind === "constant") {
        setEntityType(String(src.value ?? ""));
        continue;
      }
      if (src.kind !== "column" || !src.column) continue;
      if (known.size > 0 && !known.has(src.column)) missing.push(src.column);
      next[field as IntakeTargetField] = {
        column: src.column,
        transform: mapping.transforms?.[0]?.kind ?? "",
      };
    }

    setChoices(next);
    if (m.delimiter) setDelimiter(m.delimiter);

    // A saved database import carries its source and its schedule, so loading
    // one brings back everything it needs to run. Without this the connection
    // and query would come back empty and re-saving would quietly strip the
    // schedule off an import that was working.
    if (m.source_kind === "database") {
      setSourceKind("database");
      setDbConnection(m.db_connection ?? "");
      setDbQuery(m.db_query ?? "");
      setFilterColumn(m.filter_column ?? "");
      setFilterValue(m.filter_value ?? "");
      if (m.create_owners !== undefined) setCreateOwners(m.create_owners);
      setScheduleEnabled(Boolean(m.schedule_enabled));
      if (m.schedule) setSchedule(m.schedule);
    }

    setReport(null);
    setSaved(null);
    // A mapping naming columns this file does not have would otherwise look
    // applied and quietly map nothing.
    setLoadedMapping(
      missing.length > 0
        ? `Loaded "${m.name}", but this file has no column named ${missing.map((c) => `"${c}"`).join(", ")} — check the mapping below.`
        : `Loaded "${m.name}".`,
    );
  }

  // Where the rows come from. A file, or a query against a database — the
  // customer's owner list lives in a system of record, not always a export.
  const [sourceKind, setSourceKind] = useState<"file" | "database">("file");
  // The name of a connection set up in the panel below. The connection itself
  // carries which database reads it and where its password is kept, so nothing
  // here asks either question a second time.
  const [dbConnection, setDbConnection] = useState("");
  const [dbQuery, setDbQuery] = useState("");

  // A saved database import can run unattended. Only a database one: a file
  // import has no stored source, because somebody has to bring the file.
  const [scheduleEnabled, setScheduleEnabled] = useState(false);
  const [schedule, setSchedule] = useState("0 2 * * *");

  const [tables, setTables] = useState<IntakeDatabaseTable[] | null>(null);
  const [tableFilter, setTableFilter] = useState("");

  /** List what the connection can see, so a table can be picked rather than
   * typed. The person setting this up often cannot inspect the database. */
  async function handleBrowseTables() {
    setError(null);
    setLoading("Looking at what is there…");
    try {
      const res = await listImportDatabaseTables(dbConnection);
      setTables(res.data ?? []);
    } catch (err: unknown) {
      // An unreadable list must not render as an empty one.
      setTables(null);
      reportError(err, "Could not list the tables.");
    } finally {
      setLoading(null);
    }
  }

  /** Choosing a table writes the query for you. It stays editable — a join
   * across two tables is the common case, and this is the starting point. */
  function handleChooseTable(t: IntakeDatabaseTable) {
    setDbQuery(`SELECT * FROM ${t.qualified_name}`);
    setTables(null);
    setTableFilter("");
  }

  function databaseSource(): IntakeDatabaseSource {
    return { connection: dbConnection, query: dbQuery };
  }

  /** Read the query's columns, so the mapping below has something to offer. */
  async function handleReadQuery() {
    setProfile(null);
    setChoices({});
    setReport(null);
    setError(null);
    setSaved(null);
    setLoading("Running the query…");
    try {
      const result = await profileImportDatabase(databaseSource());
      setProfile(result);
      setChoices(guessMapping(result));
    } catch (err: unknown) {
      reportError(err, "Could not read from the database.");
    } finally {
      setLoading(null);
    }
  }

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

  /** Ready to run when the mapping is complete and a source is chosen. */
  const sourceReady =
    sourceKind === "file" ? Boolean(file) : Boolean(dbConnection && dbQuery.trim());

  const blockedReason = previewBlockedReason({
    sourceKind,
    sourceReady,
    hasOwner: Boolean(choices.owner?.column),
    hasEntityKey: Boolean(choices.entity_key?.column),
  });

  async function run(commit: boolean) {
    if (!sourceReady || !mappingComplete) return;

    setLoading(commit ? "Importing…" : "Working out what would happen…");
    setError(null);
    try {
      // Each call opens its own source; the server remembers nothing between
      // them, so preview and commit each carry the query again.
      const opts = {
        ...(sourceKind === "database"
          ? { database: databaseSource() }
          : { file: file ?? undefined }),
        delimiter: delimiter || undefined,
        fieldMap: fieldMap(),
        filterColumn: filterColumn || undefined,
        filterValue,
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
        // A database import saves what it reads as well as how it maps it, so
        // it can be re-run — and, with a schedule, re-run unattended.
        ...(sourceKind === "database"
          ? {
              source_kind: "database",
              db_connection: dbConnection,
              db_query: dbQuery,
              // The row filter is part of the import, not a convenience of
              // this screen: an unattended run that dropped it would import
              // every kind of row under the entity type chosen above.
              filter_column: filterColumn,
              filter_value: filterValue,
              // Saved because it changes what an unattended run does to the
              // owner catalogue. Left to a default, a run could do the
              // opposite of what was previewed.
              create_owners: createOwners,
              schedule: scheduleEnabled ? schedule.trim() : "",
              schedule_enabled: scheduleEnabled,
            }
          : {}),
      });
      setSaved(
        scheduleEnabled
          ? `Saved as "${mapping.name}". It will run on its schedule from now on.`
          : `Saved as "${mapping.name}". A repeat import can reuse it.`,
      );
      setSaveName("");
      fetchImportMappings()
        .then((r) => setMappings(r.data ?? []))
        .catch(() => undefined);
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
        <h3 className="card-header">1. Choose where the owners come from</h3>
        <p className="mb-3 text-sm text-gray-600">
          A file, or a query against a database. Either way CMM reads the columns
          and shows you what it found — the source does not have to be in CMM's
          format.
        </p>

        <div className="mb-4 flex gap-4">
          {(["file", "database"] as const).map((kind) => (
            <label key={kind} className="flex items-center gap-2 text-sm text-gray-700">
              <input
                type="radio"
                name="intake-source"
                checked={sourceKind === kind}
                onChange={() => {
                  setSourceKind(kind);
                  setProfile(null);
                  setChoices({});
                  setReport(null);
                  setError(null);
                }}
                className="h-4 w-4 border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              {kind === "file" ? "A file" : "A database"}
            </label>
          ))}
        </div>

        {sourceKind === "database" && (
          <div className="space-y-3">
            {/* Setting the connection up is its own act, and so is testing it.
                See journeys/ownership-connection.md: what is sent has to be
                readable, because when it fails an encrypted connection string
                leaves nothing to do but guess. */}
            <OwnershipConnectionPanel value={dbConnection} onChange={setDbConnection} />

            <label className="block text-sm text-gray-700">
              <span className="mb-1 block text-xs font-medium text-gray-500">
                Query
              </span>
              <textarea
                value={dbQuery}
                onChange={(e) => setDbQuery(e.target.value)}
                rows={6}
                spellCheck={false}
                placeholder={"SELECT owner_email, asset_kind, asset_name\nFROM asset_owner\nJOIN staff ON ..."}
                className="block w-full rounded-md border border-gray-300 px-2.5 py-1.5 font-mono text-xs"
              />
            </label>
            <p className="text-xs text-gray-500">
              Your query, run as it is written, under whatever that connection is
              allowed to read. Return one row per assignment; the columns can be
              named anything — you map them in the next step.
            </p>

            {tables !== null && (
              <div className="rounded-md border border-gray-200 bg-gray-50 p-3">
                <div className="mb-2 flex items-center justify-between gap-3">
                  <span className="text-xs font-medium text-gray-600">
                    {tables.length} table{tables.length === 1 ? "" : "s"} and views
                    this connection can see
                  </span>
                  <input
                    type="text"
                    value={tableFilter}
                    onChange={(e) => setTableFilter(e.target.value)}
                    placeholder="Filter…"
                    className="w-40 rounded-md border border-gray-300 px-2 py-1 text-xs"
                  />
                </div>
                <ul className="max-h-52 divide-y divide-gray-200 overflow-auto rounded-md border border-gray-200 bg-white">
                  {tables
                    .filter((t) =>
                      `${t.schema}.${t.name}`
                        .toLowerCase()
                        .includes(tableFilter.toLowerCase()),
                    )
                    .map((t) => (
                      <li key={`${t.schema}.${t.name}`}>
                        <button
                          type="button"
                          onClick={() => handleChooseTable(t)}
                          className="flex w-full items-center justify-between px-3 py-1.5 text-left text-xs hover:bg-blue-50"
                        >
                          <span className="font-mono">
                            {t.schema}.{t.name}
                          </span>
                          <span className="text-gray-400">{t.kind}</span>
                        </button>
                      </li>
                    ))}
                </ul>
                <p className="mt-2 text-xs text-gray-500">
                  Choosing one writes a query for you. Edit it freely — owners
                  usually need a join, and this is only the starting point.
                </p>
              </div>
            )}

            <button
              type="button"
              onClick={handleBrowseTables}
              disabled={!dbConnection || loading !== null}
              className="mr-2 rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:bg-gray-100 disabled:text-gray-400"
            >
              Browse tables
            </button>
            <button
              type="button"
              onClick={handleReadQuery}
              disabled={!dbConnection || !dbQuery.trim() || loading !== null}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:bg-gray-300"
            >
              Read the query
            </button>
          </div>
        )}

        <div
          className={`flex flex-wrap items-center gap-3 ${sourceKind === "database" ? "hidden" : ""}`}
        >
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
        {/* Guidance about a file, so it goes when the source is a database.
            The input was already hidden and this was not, which left advice
            about a control nobody could see. */}
        {sourceKind !== "database" && (
          <p className="mt-2 text-xs text-gray-500">
            CMM guesses the delimiter. If it guesses wrong, set it here and choose
            the file again — the guess is never binding.
          </p>
        )}
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
                      <td>
                        {column.distinct_count.toLocaleString()}
                        {column.distinct_capped && "+"}
                      </td>
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

            {/* A consolidated export holds several kinds of row — declared
                owners beside inferred committers. Importing one kind at a time
                beats splitting the file in a spreadsheet first. */}
            <div className="mt-4 flex flex-wrap items-end gap-3 rounded-md border border-gray-200 bg-gray-50 p-3">
              <label className="block space-y-1">
                <span className="block text-xs font-medium text-gray-700">
                  Only import rows where
                </span>
                <select
                  value={filterColumn}
                  onChange={(e) => {
                    setFilterColumn(e.target.value);
                    if (!e.target.value) setFilterValue("");
                  }}
                  className="rounded-md border border-gray-300 px-2 py-1 text-sm"
                >
                  <option value="">(no filter — import every row)</option>
                  {(profile?.columns ?? []).map((c) => (
                    <option key={c.name} value={c.name}>
                      {c.name}
                    </option>
                  ))}
                </select>
              </label>
              <label className="block space-y-1">
                <span className="block text-xs font-medium text-gray-700">
                  equals
                </span>
                <input
                  type="text"
                  value={filterValue}
                  disabled={!filterColumn}
                  onChange={(e) => setFilterValue(e.target.value)}
                  placeholder="e.g. owner"
                  className="rounded-md border border-gray-300 px-2 py-1 text-sm disabled:bg-gray-100"
                />
              </label>
              <span className="pb-1 text-xs text-gray-500">
                Case-insensitive. The preview reports how many rows were left
                out.
              </span>
            </div>

            <div className="mt-4 flex flex-wrap items-center gap-3">
              <button
                type="button"
                onClick={() => run(false)}
                disabled={blockedReason !== null}
                title={blockedReason ?? undefined}
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                Preview — nothing is saved
              </button>
              {blockedReason && (
                <span className="text-sm text-amber-700">{blockedReason}</span>
              )}
              {mappings.length > 0 && (
                <select
                  aria-label="Load a saved mapping"
                  defaultValue=""
                  onChange={(e) => {
                    const m = mappings.find((x) => String(x.id) === e.target.value);
                    if (m) applyMapping(m);
                  }}
                  className="rounded-md border border-gray-300 px-2 py-1 text-sm"
                >
                  <option value="">Load a saved mapping…</option>
                  {mappings.map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.name}
                    </option>
                  ))}
                </select>
              )}
              <input
                type="text"
                value={saveName}
                onChange={(e) => setSaveName(e.target.value)}
                placeholder="Name this import"
                className="rounded-md border border-gray-300 px-2 py-1 text-sm"
              />
              <button
                type="button"
                onClick={handleSaveMapping}
                disabled={!mappingComplete || !saveName.trim()}
                className="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
              >
                Save import
              </button>
              {saved && <span className="text-sm text-green-700">{saved}</span>}
              {loadedMapping && (
                <span className="text-sm text-blue-700">{loadedMapping}</span>
              )}
            </div>

            {/* Only a database import can be scheduled. A file import has no
                stored source to read, so offering the control here would be
                offering something that cannot work. */}
            {sourceKind === "database" && (
              <div className="mt-4 rounded-md border border-gray-200 bg-gray-50 p-3">
                <label className="flex items-center gap-2 text-sm font-medium text-gray-700">
                  <input
                    type="checkbox"
                    checked={scheduleEnabled}
                    onChange={(e) => setScheduleEnabled(e.target.checked)}
                    className="h-4 w-4 rounded border-gray-300"
                  />
                  Run this import on a schedule
                </label>
                <p className="mt-1 text-xs text-gray-500">
                  Saved with the import, so it re-reads the database on its own
                  and nobody has to remember to. It writes the same assignments
                  a manual import would, and each run is recorded.
                </p>

                {scheduleEnabled && (
                  <div className="mt-3">
                    <label
                      htmlFor="import-schedule"
                      className="block text-sm font-medium text-gray-700"
                    >
                      Schedule
                    </label>
                    <input
                      id="import-schedule"
                      type="text"
                      value={schedule}
                      onChange={(e) => setSchedule(e.target.value)}
                      placeholder="0 2 * * *"
                      className="mt-1 w-56 rounded-md border border-gray-300 px-2 py-1 font-mono text-sm"
                    />
                    <p className="mt-1 text-xs text-gray-500">
                      Cron expression with 5 space-separated fields (e.g. 0 2 *
                      * *)
                    </p>
                    {/* Says the expression back in English. An expression
                        somebody cannot read is one they cannot check, and this
                        one runs with nobody watching. */}
                    <CronDescription expression={schedule} />
                  </div>
                )}
              </div>
            )}
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
  // The authoritative figure is the server's tally over every row. The list is
  // what survived any truncation of the detail — normally the same, but the
  // two must never be presented as if the list were the count.
  const rejectedTotal = report.counts["rejected"] ?? rejected.length;

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
        {/* Rows the filter left out are not failures and are not counted as
            outcomes — but without saying so, a file of 267,000 rows reporting
            19,000 looks like most of it went missing. */}
        {(report.filtered_out ?? 0) > 0 && (
          <p className="mt-3 rounded-md border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600">
            {report.filtered_out?.toLocaleString()} row
            {report.filtered_out === 1 ? " was" : "s were"} left out by the
            filter and not imported. {report.row_count.toLocaleString()} row
            {report.row_count === 1 ? "" : "s"} matched.
          </p>
        )}
        {report.rows_truncated && (
          <p className="mt-3 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-sm text-blue-800">
            Every one of the {report.row_count.toLocaleString()} matching rows
            was processed{report.committed ? " and imported" : ""}. Only the
            per-row list below is shortened, to keep this page usable.
          </p>
        )}
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

      {rejectedTotal > 0 && (
        <div className="card">
          <h3 className="card-header">Rows not imported ({rejectedTotal})</h3>
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
              Showing the first 200 of {rejectedTotal}.
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
// Why preview is not available yet
//
// A disabled button with no reason is the same fault as an import that drops
// rows silently: the screen knows something the person does not, and they are
// left guessing which of six controls is at fault. Owner and Entity key are
// both required, and the guess cannot always find an owner column — which is
// exactly when this button greys out and nothing says so.
//
// It also covers the source, which the disabled state used to ignore: with no
// file chosen the button looked available and did nothing at all when clicked.
// ---------------------------------------------------------------------------

export function previewBlockedReason(opts: {
  sourceKind: "file" | "database";
  sourceReady: boolean;
  hasOwner: boolean;
  hasEntityKey: boolean;
}): string | null {
  if (!opts.sourceReady) {
    return opts.sourceKind === "file"
      ? "Choose a file first."
      : "Choose a stored credential and write a query first.";
  }
  const missing: string[] = [];
  if (!opts.hasOwner) missing.push("Owner");
  if (!opts.hasEntityKey) missing.push("Entity key");
  if (missing.length === 0) return null;
  return `Map ${missing.join(" and ")} first — ${
    missing.length === 1 ? "it is" : "they are"
  } required. The guess only fills a column in when it recognises the name.`;
}

// ---------------------------------------------------------------------------
// Guessing a starting mapping
//
// A guess the administrator corrects beats an empty form. It is only ever a
// starting point — every field stays editable, and nothing is applied until
// they preview it.
// ---------------------------------------------------------------------------

export function guessMapping(
  profile: IntakeSourceProfile,
): Partial<Record<IntakeTargetField, ColumnChoice>> {
  const guesses: Partial<Record<IntakeTargetField, ColumnChoice>> = {};

  // Each guess excludes the columns already taken. Without that, a table whose
  // only name-ish column is `owner_name` maps BOTH the owner and the thing
  // owned to it — every assignment imported against the owner's own name, which
  // is silent and which a four-row preview does not obviously show.
  const find = (taken: (string | undefined)[], ...needles: string[]) =>
    profile.columns.find((column) => {
      if (taken.includes(column.name)) return false;
      const name = column.name.toLowerCase();
      return needles.some((needle) => name.includes(needle));
    })?.name;

  const owner = find([], "owner", "email", "contact", "maintainer", "assignee", "user");
  if (owner) {
    guesses.owner = { column: owner, transform: "trim" };
  }

  const key = find([owner], "repo", "cookbook", "node", "entity", "name");
  if (key) {
    guesses.entity_key = { column: key, transform: "trim" };
  }

  const org = find([owner, key], "org", "tenant", "business");
  if (org) {
    guesses.organisation = { column: org, transform: "trim" };
  }

  return guesses;
}
