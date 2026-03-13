import { useState, useRef, type ChangeEvent, type DragEvent } from "react";
import { importOwnership, ApiError } from "../api";
import type { ImportResponse } from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { useAuth } from "../context/AuthContext";

// ---------------------------------------------------------------------------
// Ownership Import page — bulk import ownership assignments via CSV or JSON
// file upload. Requires operator or admin role.
// ---------------------------------------------------------------------------

type ImportFormat = "csv" | "json";

export function OwnershipImportPage() {
  const { user } = useAuth();

  const [format, setFormat] = useState<ImportFormat>("csv");
  const [file, setFile] = useState<File | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ImportResponse | null>(null);
  const [dragOver, setDragOver] = useState(false);

  const fileInputRef = useRef<HTMLInputElement>(null);

  // Role gate — only operator or admin may import
  const allowed = user?.role === "admin" || user?.role === "operator";

  if (!allowed) {
    return (
      <div className="space-y-6">
        <nav className="text-sm text-gray-500">
          <span>Ownership</span>
          <span className="mx-1">/</span>
          <span className="text-gray-800">Import</span>
        </nav>
        <ErrorAlert message="Access denied. Operator or admin role is required to import ownership data." />
      </div>
    );
  }

  function handleFormatChange(newFormat: ImportFormat) {
    setFormat(newFormat);
    setFile(null);
    setError(null);
    setResult(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  }

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    const selected = e.target.files?.[0] ?? null;
    setFile(selected);
    setError(null);
    setResult(null);
  }

  function handleDragOver(e: DragEvent<HTMLDivElement>) {
    e.preventDefault();
    e.stopPropagation();
    setDragOver(true);
  }

  function handleDragLeave(e: DragEvent<HTMLDivElement>) {
    e.preventDefault();
    e.stopPropagation();
    setDragOver(false);
  }

  function handleDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault();
    e.stopPropagation();
    setDragOver(false);

    const dropped = e.dataTransfer.files?.[0];
    if (!dropped) return;

    const expectedExt = format === "csv" ? ".csv" : ".json";
    if (!dropped.name.toLowerCase().endsWith(expectedExt)) {
      setError(`Please drop a ${expectedExt} file for the selected format.`);
      return;
    }

    setFile(dropped);
    setError(null);
    setResult(null);
  }

  function handleBrowseClick() {
    fileInputRef.current?.click();
  }

  async function handleUpload() {
    if (!file) return;

    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const response = await importOwnership(file, format);
      setResult(response);
    } catch (err: unknown) {
      const message =
        err instanceof ApiError
          ? err.message
          : err instanceof Error
            ? err.message
            : "Import failed.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }

  function handleReset() {
    setFile(null);
    setError(null);
    setResult(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  }

  const acceptExt = format === "csv" ? ".csv" : ".json";

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <nav className="text-sm text-gray-500">
        <span>Ownership</span>
        <span className="mx-1">/</span>
        <span className="text-gray-800">Import</span>
      </nav>

      {/* Header */}
      <div>
        <h2 className="text-lg font-semibold text-gray-800">
          Import Ownership Data
        </h2>
        <p className="text-sm text-gray-500">
          Bulk import ownership assignments from a CSV or JSON file.
        </p>
      </div>

      {/* Format selector */}
      <div className="card">
        <h3 className="card-header">Import Format</h3>
        <div className="flex items-center gap-6">
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="radio"
              name="format"
              value="csv"
              checked={format === "csv"}
              onChange={() => handleFormatChange("csv")}
              className="h-4 w-4 text-blue-600 focus:ring-blue-500"
            />
            <span className="text-sm font-medium text-gray-700">CSV</span>
          </label>
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="radio"
              name="format"
              value="json"
              checked={format === "json"}
              onChange={() => handleFormatChange("json")}
              className="h-4 w-4 text-blue-600 focus:ring-blue-500"
            />
            <span className="text-sm font-medium text-gray-700">JSON</span>
          </label>
        </div>
      </div>

      {/* CSV instructions */}
      {format === "csv" && (
        <div className="card">
          <h3 className="card-header">CSV Format Instructions</h3>
          <div className="space-y-3 text-sm text-gray-600">
            <p>
              Your CSV file must include the following header row as the first
              line:
            </p>
            <pre className="rounded-md bg-gray-100 px-4 py-2 text-xs font-mono text-gray-800 overflow-x-auto">
              owner,entity_type,entity_key,organisation,notes
            </pre>
            <p>
              <span className="font-medium text-gray-700">
                Valid entity types:
              </span>{" "}
              <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">node</code>,{" "}
              <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">cookbook</code>,{" "}
              <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">git_repo</code>,{" "}
              <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">role</code>,{" "}
              <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">policy</code>
            </p>
            <p className="text-xs text-gray-500">
              Maximum 10,000 rows per import. The{" "}
              <code className="rounded bg-gray-100 px-1 py-0.5 text-xs">organisation</code>{" "}
              and{" "}
              <code className="rounded bg-gray-100 px-1 py-0.5 text-xs">notes</code>{" "}
              columns are optional.
            </p>
          </div>
        </div>
      )}

      {/* JSON instructions */}
      {format === "json" && (
        <div className="card">
          <h3 className="card-header">JSON Format Instructions</h3>
          <div className="space-y-3 text-sm text-gray-600">
            <p>
              Your JSON file should contain an array of assignment objects with
              the following structure:
            </p>
            <pre className="rounded-md bg-gray-100 px-4 py-3 text-xs font-mono text-gray-800 overflow-x-auto">
              {`[
  {
    "owner": "platform-team",
    "entity_type": "cookbook",
    "entity_key": "nginx",
    "organisation": "prod-org",
    "notes": "Primary maintainer"
  },
  {
    "owner": "platform-team",
    "entity_type": "node",
    "entity_key": "web-server-01",
    "organisation": "prod-org"
  }
]`}
            </pre>
            <p>
              <span className="font-medium text-gray-700">
                Valid entity types:
              </span>{" "}
              <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">node</code>,{" "}
              <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">cookbook</code>,{" "}
              <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">git_repo</code>,{" "}
              <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">role</code>,{" "}
              <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">policy</code>
            </p>
            <p className="text-xs text-gray-500">
              Maximum 10,000 entries per import. The{" "}
              <code className="rounded bg-gray-100 px-1 py-0.5 text-xs">organisation</code>{" "}
              and{" "}
              <code className="rounded bg-gray-100 px-1 py-0.5 text-xs">notes</code>{" "}
              fields are optional.
            </p>
          </div>
        </div>
      )}

      {/* File upload area */}
      {!result && (
        <div className="card">
          <h3 className="card-header">Upload File</h3>

          {/* Hidden file input */}
          <input
            ref={fileInputRef}
            type="file"
            accept={acceptExt}
            onChange={handleFileChange}
            className="hidden"
          />

          {/* Drag-and-drop zone */}
          <div
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            onClick={handleBrowseClick}
            className={
              "flex cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed px-6 py-10 transition-colors " +
              (dragOver
                ? "border-blue-400 bg-blue-50"
                : "border-gray-300 bg-gray-50 hover:border-gray-400 hover:bg-gray-100")
            }
          >
            <svg
              className="h-10 w-10 text-gray-400"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M3 16.5v2.25A2.25 2.25 0 0 0 5.25 21h13.5A2.25 2.25 0 0 0 21 18.75V16.5m-13.5-9L12 3m0 0 4.5 4.5M12 3v13.5"
              />
            </svg>
            {file ? (
              <p className="text-sm font-medium text-gray-700">
                Selected:{" "}
                <span className="text-blue-600">{file.name}</span>{" "}
                <span className="text-gray-400">
                  ({(file.size / 1024).toFixed(1)} KB)
                </span>
              </p>
            ) : (
              <>
                <p className="text-sm font-medium text-gray-600">
                  Drag and drop your{" "}
                  <span className="font-semibold">{acceptExt}</span> file here
                </p>
                <p className="text-xs text-gray-400">
                  or click to browse
                </p>
              </>
            )}
          </div>

          {/* Error */}
          {error && (
            <div className="mt-4">
              <ErrorAlert message={error} />
            </div>
          )}

          {/* Upload button */}
          <div className="mt-4 flex items-center gap-3">
            <button
              type="button"
              onClick={handleUpload}
              disabled={!file || loading}
              className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {loading && (
                <svg
                  className="h-4 w-4 animate-spin"
                  xmlns="http://www.w3.org/2000/svg"
                  fill="none"
                  viewBox="0 0 24 24"
                  aria-hidden="true"
                >
                  <circle
                    className="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    strokeWidth="4"
                  />
                  <path
                    className="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                  />
                </svg>
              )}
              {loading ? "Uploading…" : "Upload & Import"}
            </button>
            {file && !loading && (
              <button
                type="button"
                onClick={handleReset}
                className="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
              >
                Clear
              </button>
            )}
          </div>
        </div>
      )}

      {/* Loading overlay */}
      {loading && <LoadingSpinner message="Importing ownership data…" />}

      {/* Results */}
      {result && !loading && (
        <div className="space-y-6">
          {/* Summary */}
          <div className="card">
            <h3 className="card-header">Import Results</h3>
            <div className="flex flex-wrap gap-6">
              <div className="flex flex-col">
                <span className="text-sm font-medium text-gray-500">
                  Imported
                </span>
                <span className="mt-1 text-2xl font-bold text-green-600">
                  {result.imported}
                </span>
              </div>
              <div className="flex flex-col">
                <span className="text-sm font-medium text-gray-500">
                  Skipped
                </span>
                <span className="mt-1 text-2xl font-bold text-amber-600">
                  {result.skipped}
                </span>
              </div>
              <div className="flex flex-col">
                <span className="text-sm font-medium text-gray-500">
                  Errors
                </span>
                <span className="mt-1 text-2xl font-bold text-red-600">
                  {result.errors.length}
                </span>
              </div>
            </div>
          </div>

          {/* Error table */}
          {result.errors.length > 0 && (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <th>Line</th>
                    <th>Error Message</th>
                  </tr>
                </thead>
                <tbody>
                  {result.errors.map((err, idx) => (
                    <tr key={idx}>
                      <td className="font-mono">{err.line}</td>
                      <td className="whitespace-normal text-red-700">
                        {err.error}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Reset button */}
          <div>
            <button
              type="button"
              onClick={handleReset}
              className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
            >
              <svg
                className="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                strokeWidth={1.5}
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182"
                />
              </svg>
              Reset & Start Over
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
