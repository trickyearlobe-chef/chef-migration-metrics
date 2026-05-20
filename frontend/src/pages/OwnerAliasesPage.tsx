import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { ApiError, createOwnerAlias, deleteOwnerAlias, fetchOwnerAliases, importOwnerAliases, suggestOwnerAliases } from "../api";
import type { AliasImportResponse, AliasSuggestion, OwnerAlias } from "../types";
import { useAuth } from "../context/AuthContext";
import { EmptyState, ErrorAlert, LoadingSpinner } from "../components/Feedback";

const INPUT_CLASS = "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500";
const SELECT_CLASS = "block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500";
const FILE_INPUT_CLASS = `${INPUT_CLASS} file:mr-3 file:rounded-md file:border-0 file:bg-gray-100 file:px-3 file:py-2 file:text-sm file:font-medium file:text-gray-700`;
const ALIAS_TYPES = ["email", "name", "username", "saml_id"];
const ALIAS_STYLES: Record<string, string> = { email: "bg-sky-100 text-sky-700", name: "bg-violet-100 text-violet-700", username: "bg-amber-100 text-amber-700", saml_id: "bg-emerald-100 text-emerald-700" };
const TABS = [{ key: "browse", label: "Browse" }, { key: "import", label: "Import" }] as const;
const STATS = [
  { key: "imported", label: "Imported", className: "text-green-600" },
  { key: "skipped", label: "Skipped", className: "text-amber-600" },
  { key: "errors", label: "Errors", className: "text-red-600" },
] as const;

const errorMessage = (error: unknown, fallback: string) => error instanceof ApiError ? error.message : error instanceof Error ? error.message : fallback;
const formatSimilarity = (similarity: number) => `${(similarity <= 1 ? similarity * 100 : similarity).toFixed(similarity <= 0.1 ? 1 : 0)}%`;
const formatTimestamp = (value: string) => new Date(value).toLocaleString();

function TypeBadge({ value }: { value: string }) {
  return <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${ALIAS_STYLES[value] ?? "bg-gray-100 text-gray-700"}`}>{value}</span>;
}

export function OwnerAliasesPage() {
  const { isOperator } = useAuth();
  const [searchParams] = useSearchParams();
  const initialOwner = searchParams.get("owner") ?? "";
  const [activeTab, setActiveTab] = useState<"browse" | "import">("browse");
  const [searchQuery, setSearchQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [suggestions, setSuggestions] = useState<AliasSuggestion[]>([]);
  const [suggestionsLoading, setSuggestionsLoading] = useState(false);
  const [suggestionsError, setSuggestionsError] = useState<string | null>(null);
  const [ownerInput, setOwnerInput] = useState(initialOwner);
  const [selectedOwner, setSelectedOwner] = useState("");
  const [aliases, setAliases] = useState<OwnerAlias[]>([]);
  const [aliasesLoading, setAliasesLoading] = useState(false);
  const [aliasesError, setAliasesError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState<string | null>(null);
  const [savingAlias, setSavingAlias] = useState(false);
  const [deletingAliasId, setDeletingAliasId] = useState<string | null>(null);
  const [aliasForm, setAliasForm] = useState({ owner_name: "", alias_type: "email", alias_value: "" });
  const [importFormat, setImportFormat] = useState<"csv" | "json">("csv");
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importLoading, setImportLoading] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const [importResult, setImportResult] = useState<AliasImportResponse | null>(null);

  const loadAliases = useCallback(async (ownerName: string) => {
    const trimmed = ownerName.trim();
    if (!trimmed) {
      setSelectedOwner("");
      setAliases([]);
      setAliasesError(null);
      return;
    }
    setAliasesLoading(true);
    setAliasesError(null);
    try {
      const response = await fetchOwnerAliases(trimmed);
      setAliases(response.aliases ?? []);
      setSelectedOwner(trimmed);
      setOwnerInput(trimmed);
      setAliasForm((current) => ({ ...current, owner_name: current.owner_name || trimmed }));
    } catch (error: unknown) {
      setAliasesError(errorMessage(error, "Failed to load aliases."));
    } finally {
      setAliasesLoading(false);
    }
  }, []);

  useEffect(() => {
    const timeoutId = window.setTimeout(() => setDebouncedQuery(searchQuery.trim()), 300);
    return () => window.clearTimeout(timeoutId);
  }, [searchQuery]);

  useEffect(() => {
    if (!debouncedQuery) {
      setSuggestions([]);
      setSuggestionsError(null);
      setSuggestionsLoading(false);
      return;
    }
    let cancelled = false;
    setSuggestionsLoading(true);
    setSuggestionsError(null);
    suggestOwnerAliases(debouncedQuery, 8)
      .then((response) => !cancelled && setSuggestions(response.suggestions ?? []))
      .catch((error: unknown) => !cancelled && setSuggestionsError(errorMessage(error, "Failed to load suggestions.")))
      .finally(() => !cancelled && setSuggestionsLoading(false));
    return () => {
      cancelled = true;
    };
  }, [debouncedQuery]);

  // Auto-load aliases if navigated with ?owner= query param.
  useEffect(() => {
    if (initialOwner) {
      loadAliases(initialOwner);
    }
  }, [initialOwner, loadAliases]);

  async function handleBrowseSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await loadAliases(ownerInput);
  }

  async function handleSuggestionSelect(suggestion: AliasSuggestion) {
    setOwnerInput(suggestion.owner_name);
    setAliasForm((current) => ({ ...current, owner_name: suggestion.owner_name }));
    await loadAliases(suggestion.owner_name);
  }

  async function handleAddAlias(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSavingAlias(true);
    setSaveError(null);
    setSaveSuccess(null);
    const body = { owner_name: aliasForm.owner_name.trim(), alias_type: aliasForm.alias_type, alias_value: aliasForm.alias_value.trim() };
    try {
      await createOwnerAlias(body);
      setSaveSuccess("Alias added.");
      setAliasForm((current) => ({ ...current, alias_value: "" }));
      await loadAliases(body.owner_name);
    } catch (error: unknown) {
      setSaveError(errorMessage(error, "Failed to add alias."));
    } finally {
      setSavingAlias(false);
    }
  }

  async function handleDeleteAlias(alias: OwnerAlias) {
    setDeletingAliasId(alias.id);
    setAliasesError(null);
    try {
      await deleteOwnerAlias(alias.id);
      await loadAliases(alias.owner_name);
    } catch (error: unknown) {
      setAliasesError(errorMessage(error, "Failed to delete alias."));
    } finally {
      setDeletingAliasId(null);
    }
  }

  async function handleImportSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!importFile) return;
    setImportLoading(true);
    setImportError(null);
    setImportResult(null);
    try {
      setImportResult(await importOwnerAliases(importFile, importFormat));
    } catch (error: unknown) {
      setImportError(errorMessage(error, "Failed to import aliases."));
    } finally {
      setImportLoading(false);
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Owner Aliases</h2>
        <p className="mt-1 text-sm text-gray-500">Search for likely owner matches, review aliases by owner, and manage imported identity mappings.</p>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
        <label className="mb-2 block text-sm font-medium text-gray-700">Search alias suggestions</label>
        <input type="search" value={searchQuery} onChange={(event) => setSearchQuery(event.target.value)} placeholder="Search by alias value, owner name, email, or username" className={INPUT_CLASS} />
        {suggestionsError && <div className="mt-3"><ErrorAlert message={suggestionsError} /></div>}
        {suggestionsLoading && <p className="mt-3 text-sm text-gray-500">Searching…</p>}
        {!suggestionsLoading && !suggestionsError && debouncedQuery && suggestions.length === 0 && (
          <div className="mt-3 rounded-md border border-dashed border-gray-200 bg-gray-50 px-4 py-6">
            <EmptyState title="No suggestions found" description="Try a broader query or browse aliases by owner below." />
          </div>
        )}
        {suggestions.length > 0 && (
          <div className="mt-3 overflow-hidden rounded-lg border border-gray-200">
            <ul className="divide-y divide-gray-200">
              {suggestions.map((suggestion, index) => (
                <li key={`${suggestion.owner_name}-${suggestion.alias_value}-${index}`}>
                  <button type="button" onClick={() => void handleSuggestionSelect(suggestion)} className="flex w-full items-center justify-between gap-4 px-4 py-3 text-left transition-colors hover:bg-gray-50">
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-gray-900">{suggestion.owner_name}</span>
                        <TypeBadge value={suggestion.alias_type} />
                      </div>
                      <p className="mt-1 text-sm text-gray-600">{suggestion.alias_value}</p>
                    </div>
                    <span className="text-xs font-medium text-gray-500">{formatSimilarity(suggestion.similarity)}</span>
                  </button>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>

      <div className="border-b border-gray-200">
        <nav className="flex gap-4">
          {TABS.map((tab) => (
            <button key={tab.key} type="button" onClick={() => setActiveTab(tab.key)} className={`border-b-2 px-1 py-3 text-sm font-medium ${activeTab === tab.key ? "border-blue-600 text-blue-600" : "border-transparent text-gray-500 hover:text-gray-700"}`}>
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {activeTab === "browse" && (
        <div className="space-y-6">
          <div className={`grid gap-6 ${isOperator ? "lg:grid-cols-2" : "lg:grid-cols-1"}`}>
            <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
              <h3 className="text-sm font-semibold text-gray-900">Browse aliases by owner</h3>
              <form onSubmit={handleBrowseSubmit} className="mt-4 flex gap-3">
                <input type="text" value={ownerInput} onChange={(event) => setOwnerInput(event.target.value)} placeholder="owner-name" className={INPUT_CLASS} />
                <button type="submit" disabled={!ownerInput.trim() || aliasesLoading} className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:opacity-50">Browse</button>
              </form>
              <p className="mt-2 text-xs text-gray-500">Enter an owner name to review all known aliases.</p>
            </div>

            {isOperator && (
              <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
                <h3 className="text-sm font-semibold text-gray-900">Add Alias</h3>
                <form onSubmit={handleAddAlias} className="mt-4 space-y-4">
                  <div>
                    <label className="mb-1 block text-xs font-medium text-gray-700">Owner name</label>
                    <input type="text" value={aliasForm.owner_name} onChange={(event) => setAliasForm((current) => ({ ...current, owner_name: event.target.value }))} className={INPUT_CLASS} required />
                  </div>
                  <div className="grid gap-4 sm:grid-cols-2">
                    <div>
                      <label className="mb-1 block text-xs font-medium text-gray-700">Alias type</label>
                      <select value={aliasForm.alias_type} onChange={(event) => setAliasForm((current) => ({ ...current, alias_type: event.target.value }))} className={SELECT_CLASS}>
                        {ALIAS_TYPES.map((option) => <option key={option} value={option}>{option}</option>)}
                      </select>
                    </div>
                    <div>
                      <label className="mb-1 block text-xs font-medium text-gray-700">Alias value</label>
                      <input type="text" value={aliasForm.alias_value} onChange={(event) => setAliasForm((current) => ({ ...current, alias_value: event.target.value }))} className={INPUT_CLASS} required />
                    </div>
                  </div>
                  {saveError && <ErrorAlert message={saveError} />}
                  {saveSuccess && <div className="rounded-md border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-700">{saveSuccess}</div>}
                  <button type="submit" disabled={savingAlias || !aliasForm.owner_name.trim() || !aliasForm.alias_value.trim()} className="rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-green-700 disabled:opacity-50">
                    {savingAlias ? "Saving…" : "Add Alias"}
                  </button>
                </form>
              </div>
            )}
          </div>

          {aliasesLoading && <LoadingSpinner message="Loading aliases…" />}
          {!aliasesLoading && aliasesError && <ErrorAlert message={aliasesError} onRetry={() => void loadAliases(selectedOwner || ownerInput)} />}
          {!aliasesLoading && !aliasesError && !selectedOwner && <div className="rounded-lg border border-dashed border-gray-200 bg-white p-6 shadow-sm"><EmptyState title="No owner selected" description="Browse an owner or pick a suggestion to view aliases." /></div>}
          {!aliasesLoading && !aliasesError && selectedOwner && (
            <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
              <div className="border-b border-gray-100 px-4 py-3"><h3 className="text-sm font-semibold text-gray-900">Aliases for {selectedOwner}</h3></div>
              <div className="p-4">
                {aliases.length === 0 ? <EmptyState title="No aliases found" description="This owner does not have any aliases yet." /> : (
                  <div className="space-y-3">
                    {aliases.map((alias) => (
                      <div key={alias.id} className="flex flex-col gap-3 rounded-lg border border-gray-200 p-4 sm:flex-row sm:items-center sm:justify-between">
                        <div className="space-y-1">
                          <div className="flex items-center gap-2"><TypeBadge value={alias.alias_type} /><span className="font-medium text-gray-900">{alias.alias_value}</span></div>
                          <div className="flex flex-wrap gap-3 text-xs text-gray-500"><span>Source: {alias.source}</span><span>Created: {formatTimestamp(alias.created_at)}</span></div>
                        </div>
                        {isOperator && <button type="button" onClick={() => void handleDeleteAlias(alias)} disabled={deletingAliasId === alias.id} className="rounded-md border border-red-200 bg-white px-3 py-2 text-sm font-medium text-red-600 shadow-sm hover:bg-red-50 disabled:opacity-50">{deletingAliasId === alias.id ? "Deleting…" : "Delete"}</button>}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      )}

      {activeTab === "import" && (
        <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
          {!isOperator ? <ErrorAlert message="Operator or admin role is required to import aliases." /> : (
            <form onSubmit={handleImportSubmit} className="space-y-4">
              <div>
                <h3 className="text-sm font-semibold text-gray-900">Bulk Import</h3>
                <p className="mt-1 text-sm text-gray-500">Upload a CSV or JSON file to create aliases in bulk.</p>
              </div>
              <div className="grid gap-4 md:grid-cols-[200px_minmax(0,1fr)]">
                <div>
                  <label className="mb-1 block text-xs font-medium text-gray-700">Format</label>
                  <select value={importFormat} onChange={(event) => setImportFormat(event.target.value as "csv" | "json")} className={SELECT_CLASS}><option value="csv">csv</option><option value="json">json</option></select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-gray-700">File</label>
                  <input type="file" accept={importFormat === "csv" ? ".csv" : ".json"} onChange={(event) => setImportFile(event.target.files?.[0] ?? null)} className={FILE_INPUT_CLASS} />
                </div>
              </div>
              {importError && <ErrorAlert message={importError} />}
              <div className="flex flex-wrap items-center gap-3">
                <button type="submit" disabled={!importFile || importLoading} className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:opacity-50">{importLoading ? "Importing…" : "Import Aliases"}</button>
                {importFile && <span className="text-sm text-gray-500">{importFile.name}</span>}
              </div>
              {importLoading && <LoadingSpinner message="Importing aliases…" />}
              {importResult && !importLoading && (
                <div className="space-y-4 rounded-lg border border-gray-200 bg-gray-50 p-4">
                  <div className="grid gap-3 sm:grid-cols-3">
                    {STATS.map((stat) => {
                      const value = stat.key === "errors" ? importResult.errors?.length ?? 0 : importResult[stat.key];
                      return <div key={stat.key} className="rounded-md bg-white p-3 shadow-sm"><p className="text-xs font-medium text-gray-500">{stat.label}</p><p className={`mt-1 text-2xl font-semibold ${stat.className}`}>{value}</p></div>;
                    })}
                  </div>
                  {importResult.errors && importResult.errors.length > 0 && (
                    <div className="overflow-hidden rounded-lg border border-gray-200 bg-white">
                      <table className="table">
                        <thead><tr><th>Line</th><th>Error</th></tr></thead>
                        <tbody>
                          {importResult.errors.map((entry) => (
                            <tr key={`${entry.line}-${entry.error}`}><td>{entry.line}</td><td className="whitespace-normal">{entry.error}</td></tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              )}
            </form>
          )}
        </div>
      )}
    </div>
  );
}
