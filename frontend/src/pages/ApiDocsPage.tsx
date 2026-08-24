// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import type React from "react";
import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { fetchApiDocument } from "../api";
import type { ApiOperation, ApiSchema, OpenApiDocument } from "../types";
import { ErrorAlert, LoadingSpinner } from "../components/Feedback";

// The service's own description, rendered for a person rather than a client
// generator. Read-only by construction: there is no control here that calls
// anything, because the page is served to a signed-in person and a "try" button
// would fire real requests as them. The curl example is the deliberate
// substitute — it puts the call in the reader's own terminal, where they own
// what happens.
//
// Hand-rolled rather than a renderer library: the cheapest of the real ones
// adds a large transitive dependency tree.

const METHOD_ORDER = ["get", "post", "put", "patch", "delete", "head", "options"];

const METHOD_STYLE: Record<string, string> = {
  get: "bg-sky-100 text-sky-800",
  post: "bg-emerald-100 text-emerald-800",
  put: "bg-amber-100 text-amber-800",
  patch: "bg-amber-100 text-amber-800",
  delete: "bg-red-100 text-red-800",
};

const ROLE_STYLE: Record<string, string> = {
  admin: "bg-purple-100 text-purple-700",
  operator: "bg-blue-100 text-blue-700",
  authenticated: "bg-gray-100 text-gray-600",
  public: "bg-green-100 text-green-700",
};

const READ_METHODS = ["get", "head", "options"];

// Both ends are bounded so neither pane can be dragged away to nothing: the
// list stays usable, and the panel never becomes a sliver that wraps every
// line of the curl example.
const PANEL_MIN_WIDTH = 320;
const PANEL_MAX_WIDTH = 900;
const PANEL_DEFAULT_WIDTH = 480;

function clampWidth(w: number): number {
  return Math.min(PANEL_MAX_WIDTH, Math.max(PANEL_MIN_WIDTH, Math.round(w)));
}

// Where the chosen width is kept. Browser-side only — it never reaches the
// service, so there is nothing here to protect and nothing for git to see.
const PANEL_WIDTH_KEY = "cmm.apiDocs.panelWidth";

// Reading is defensive on both counts: storage can be unavailable outright
// (private browsing, a locked-down profile), and what is in it can be junk
// left by a hand edit or by an older build with different bounds. Either way
// the default is the right answer, not an error.
function storedPanelWidth(): number {
  try {
    const raw = window.localStorage.getItem(PANEL_WIDTH_KEY);
    if (raw === null) return PANEL_DEFAULT_WIDTH;
    const parsed = Number.parseInt(raw, 10);
    return Number.isFinite(parsed) ? clampWidth(parsed) : PANEL_DEFAULT_WIDTH;
  } catch {
    return PANEL_DEFAULT_WIDTH;
  }
}

function rememberPanelWidth(width: number): void {
  try {
    window.localStorage.setItem(PANEL_WIDTH_KEY, String(width));
  } catch {
    // Not being able to remember a pane width is not worth telling anybody
    // about, and definitely not worth failing the page over.
  }
}

interface Entry {
  path: string;
  method: string;
  op: ApiOperation;
}

// groupOf names the area an address belongs to, taken from the path because the
// document carries no tags. /api/v1/admin/... groups as "admin" rather than by
// what follows it, so everything an administrator touches reads as one section.
function groupOf(path: string): string {
  const parts = path.replace(/^\/api\/v\d+\//, "").split("/");
  return parts[0] || "root";
}

function entriesOf(doc: OpenApiDocument | null): Entry[] {
  if (!doc?.paths) return [];
  const out: Entry[] = [];
  for (const [path, item] of Object.entries(doc.paths)) {
    for (const [method, op] of Object.entries(item ?? {})) {
      if (!op) continue;
      out.push({ path, method: method.toLowerCase(), op });
    }
  }
  return out.sort(
    (a, b) =>
      a.path.localeCompare(b.path) ||
      METHOD_ORDER.indexOf(a.method) - METHOD_ORDER.indexOf(b.method),
  );
}

type Schemas = Record<string, ApiSchema>;

// resolve follows a reference to the type it names. Named types are emitted
// once under components and referred to everywhere else, so a caller reading
// "$ref: #/components/schemas/webapi.loginRequest" has been told nothing —
// every reader of a schema has to follow it or render our internal type name at
// somebody.
//
// A reference that does not resolve returns undefined rather than throwing: a
// dangling one is a bug in the generator, and the page saying "not described"
// is a better failure than a blank screen.
function resolve(schema: ApiSchema | undefined, schemas: Schemas): ApiSchema | undefined {
  let current = schema;
  // Bounded rather than recursive: a reference cycle in the document must not
  // hang the browser.
  for (let hops = 0; current?.$ref && hops < 20; hops++) {
    current = schemas[current.$ref.replace("#/components/schemas/", "")];
  }
  return current?.$ref ? undefined : current;
}

// allOf is how one body read into several types is described. Flattening the
// parts into one field list is what a caller needs — they send one document,
// not three.
function fieldsOf(
  schema: ApiSchema | undefined,
  schemas: Schemas,
): Array<[string, ApiSchema]> {
  const target = resolve(schema, schemas);
  if (!target) return [];
  if (target.allOf) {
    return target.allOf.flatMap((part) => fieldsOf(part, schemas));
  }
  return Object.entries(target.properties ?? {});
}

// typeLabel says what a field holds, in the reader's terms rather than JSON
// Schema's. An unresolved or untyped schema reads as "any", which is honest:
// the generator emits exactly that for the places where the service genuinely
// accepts anything.
function typeLabel(schema: ApiSchema | undefined, schemas: Schemas): string {
  const target = resolve(schema, schemas);
  if (!target) return "any";
  if (target.allOf) return "object";
  if (target.type === "array") {
    return `${typeLabel(target.items, schemas)}[]`;
  }
  if (target.type === "object" && target.additionalProperties) {
    return `map of ${typeLabel(target.additionalProperties, schemas)}`;
  }
  if (!target.type) return "any";
  return target.format ? `${target.type} (${target.format})` : target.type;
}

// exampleFor builds a document with every field in it, so the curl below is a
// call somebody can edit rather than a shape they have to look up. Values are
// obviously-placeholder rather than plausible: a realistic-looking example gets
// pasted and sent.
function exampleFor(schema: ApiSchema | undefined, schemas: Schemas, depth = 0): unknown {
  const target = resolve(schema, schemas);
  if (!target || depth > 4) return null;
  if (target.allOf) {
    return Object.assign({}, ...target.allOf.map((p) => exampleFor(p, schemas, depth)));
  }
  if (target.properties) {
    const out: Record<string, unknown> = {};
    for (const [name, field] of Object.entries(target.properties)) {
      out[name] = exampleFor(field, schemas, depth + 1);
    }
    return out;
  }
  switch (target.type) {
    case "array":
      return [exampleFor(target.items, schemas, depth + 1)];
    case "object":
      return {};
    case "string":
      return target.format === "date-time" ? "2026-01-01T00:00:00Z" : "";
    case "integer":
    case "number":
      return 0;
    case "boolean":
      return false;
    default:
      return null;
  }
}

// limitsOf spells out the bounds a parameter carries. The maximum matters more
// than it looks: asking for more than the cap is not refused, it quietly
// returns the cap — so a caller who cannot see the number writes a loop that
// silently reads the same page forever.
function limitsOf(schema: ApiSchema | undefined): string {
  if (!schema) return "";
  const parts: string[] = [];
  if (schema.minimum !== undefined || schema.maximum !== undefined) {
    parts.push(`${schema.minimum ?? ""}\u2013${schema.maximum ?? ""}`);
  }
  if (schema.default !== undefined) {
    parts.push(`default ${schema.default}`);
  }
  return parts.length ? `(${parts.join(", ")})` : "";
}

// jsonBodySchema is the JSON document a call takes, if it takes one. An upload
// is deliberately not one: its content type is different and its fields are not
// described, so treating it as JSON would put an empty object in the curl and
// send somebody to a refusal.
function jsonBodySchema(entry: Entry): ApiSchema | undefined {
  return entry.op.requestBody?.content?.["application/json"]?.schema;
}

// curlFor builds a command the reader can paste. The token is a shell variable,
// never a literal: an example carrying a real credential is a credential that
// ends up in a ticket, a wiki and a screenshot.
//
// Path variables become upper-case placeholders so an unsubstituted one fails
// loudly at the server rather than quietly matching something.
function curlFor(entry: Entry, origin: string, schemas: Schemas): string {
  const path = entry.path.replace(/\{([^}]+)\}/g, (_, name: string) =>
    name.toUpperCase().replace(/[^A-Z0-9]/g, "_"),
  );
  const lines = ["curl -sS \\"];
  if (!READ_METHODS.includes(entry.method)) {
    lines.push(`  -X ${entry.method.toUpperCase()} \\`);
  }
  lines.push(`  -H 'Authorization: Bearer $APITOKEN' \\`);

  const body = jsonBodySchema(entry);
  const isUpload = entry.op.requestBody?.content?.["multipart/form-data"] !== undefined;
  if (body) {
    const example = exampleFor(body, schemas);
    const rendered = JSON.stringify(example ?? {}, null, 2)
      .split("\n")
      .join("\n  ");
    lines.push(`  -H 'Content-Type: application/json' \\`);
    lines.push(`  -d '${rendered}' \\`);
  } else if (isUpload) {
    // The field names are not described yet, so naming one here would be an
    // invention. Showing the shape of the call is still worth more than
    // showing nothing.
    lines.push(`  -F 'file=@your-file' \\`);
  }
  lines.push(`  '${origin}${path}'`);
  return lines.join("\n");
}

// FieldRows renders a body's fields, descending into nested objects so a
// caller sees the whole document rather than a row saying "object" with no way
// to find out what goes in it.
function FieldRows({
  fields,
  schemas,
  depth = 0,
}: {
  fields: Array<[string, ApiSchema]>;
  schemas: Schemas;
  depth?: number;
}) {
  return (
    <>
      {fields.map(([name, field]) => {
        // An array of objects is described by its element, which is what the
        // caller actually fills in.
        const resolved = resolve(field, schemas);
        const nestedSource =
          resolved?.type === "array" ? resolved.items : field;
        // Bounded so a self-referencing type cannot render forever.
        const nested = depth < 4 ? fieldsOf(nestedSource, schemas) : [];
        return (
          <Fragment key={name}>
            <tr className="border-t border-gray-100">
              <td
                className="py-1 font-mono text-gray-900"
                style={{ paddingLeft: depth * 12 }}
              >
                {name}
              </td>
              <td className="py-1 text-gray-600">{typeLabel(field, schemas)}</td>
            </tr>
            {nested.length > 0 && (
              <FieldRows fields={nested} schemas={schemas} depth={depth + 1} />
            )}
          </Fragment>
        );
      })}
    </>
  );
}

// responseSchema is what an operation says it answers with, or undefined where
// it says nothing. Undescribed and "answers nothing" are different claims, and
// the panel below is careful to make only the first one.
function responseSchema(entry: Entry): ApiSchema | undefined {
  return entry.op.responses?.["200"]?.content?.["application/json"]?.schema;
}

// describesShape reports whether the panel can say anything about a schema.
//
// Asking only "does it have fields?" reads every list, every map and every
// one-of as undescribed, so a body the description genuinely spells out as a
// list of strings reads as unknown. Everything the generator can emit has to
// have an answer here; ApiDocsPage.contract.test.ts holds that against what the
// service really emits.
export function describesShape(
  schema: ApiSchema | undefined,
  schemas: Schemas,
): boolean {
  const target = resolve(schema, schemas);
  if (!target) return false;
  if (target.oneOf?.length || target.allOf?.length) return true;
  if (target.properties && Object.keys(target.properties).length > 0) return true;
  if (target.type === "array") return describesShape(target.items, schemas) ||
    target.items?.type !== undefined;
  if (target.additionalProperties) return true;
  // A bare type — string, integer, boolean — is a complete description of a
  // scalar answer. An empty schema is not: it means "anything", and saying so
  // is a different sentence, written where it is used.
  return target.type !== undefined;
}

// SchemaShape renders whatever a schema is: a table of fields, a list with its
// element described, a map, or one of several shapes. Shared by the request and
// response sections so neither can quietly describe less than the other.
function SchemaShape({
  schema,
  schemas,
  testId,
}: {
  schema: ApiSchema | undefined;
  schemas: Schemas;
  testId: string;
}) {
  const target = resolve(schema, schemas);
  if (!target) return null;

  if (target.oneOf?.length) {
    return (
      <div data-testid={testId}>
        <p className="mt-1 text-sm text-gray-600">
          One of {target.oneOf.length} shapes, depending on what was asked for.
        </p>
        {target.oneOf.map((part, i) => (
          <SchemaShape key={i} schema={part} schemas={schemas} testId={`${testId}-${i}`} />
        ))}
      </div>
    );
  }

  if (target.type === "array") {
    const inner = resolve(target.items, schemas);
    const fields = fieldsOf(target.items, schemas);
    return (
      <div data-testid={testId}>
        <p className="mt-1 text-sm text-gray-600">
          A list of <code className="font-mono">{typeLabel(target.items, schemas)}</code>.
        </p>
        {fields.length > 0 && <FieldTable fields={fields} schemas={schemas} />}
        {fields.length === 0 && inner?.additionalProperties && (
          <p className="mt-1 text-sm text-gray-600">
            Each entry is a map of{" "}
            <code className="font-mono">
              {typeLabel(inner.additionalProperties, schemas)}
            </code>
            .
          </p>
        )}
      </div>
    );
  }

  const fields = fieldsOf(schema, schemas);
  if (fields.length > 0) {
    return (
      <div data-testid={testId}>
        <FieldTable fields={fields} schemas={schemas} />
      </div>
    );
  }

  if (target.additionalProperties) {
    return (
      <p data-testid={testId} className="mt-1 text-sm text-gray-600">
        A map of{" "}
        <code className="font-mono">
          {typeLabel(target.additionalProperties, schemas)}
        </code>
        , keyed by whatever the answer holds.
      </p>
    );
  }

  if (target.type) {
    return (
      <p data-testid={testId} className="mt-1 text-sm text-gray-600">
        A single <code className="font-mono">{typeLabel(schema, schemas)}</code>.
      </p>
    );
  }
  return null;
}

// FieldTable is the two-column table both sections render fields with.
function FieldTable({
  fields,
  schemas,
}: {
  fields: Array<[string, ApiSchema]>;
  schemas: Schemas;
}) {
  return (
    <table className="mt-1.5 w-full text-left text-sm">
      <thead>
        <tr className="text-xs uppercase tracking-wider text-gray-400">
          <th className="pb-1 font-medium">Field</th>
          <th className="pb-1 font-medium">Type</th>
        </tr>
      </thead>
      <tbody>
        <FieldRows fields={fields} schemas={schemas} />
      </tbody>
    </table>
  );
}

// RequestBodySection says one of four things, and the difference between them
// matters more than any of them individually: here are the fields, this is a
// file upload, this takes a body we deliberately do not describe, or this
// genuinely reads nothing. An empty section would read as the last one whatever
// the truth was, which is how somebody ends up at a 400 they cannot explain.
function RequestBodySection({ entry, schemas }: { entry: Entry; schemas: Schemas }) {
  const body = entry.op.requestBody;

  if (!body) {
    return (
      <div>
        <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-500">
          Body parameters
        </h3>
        <p className="mt-1 text-sm text-gray-500">
          None — this call reads nothing from the body. What it acts on is in
          the address.
        </p>
      </div>
    );
  }

  const schema = jsonBodySchema(entry);

  return (
    <div>
      <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-500">
        Body parameters
      </h3>
      {body.description && (
        <p className="mt-1 rounded border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
          {body.description}
        </p>
      )}
      {describesShape(schema, schemas) ? (
        <SchemaShape schema={schema} schemas={schemas} testId="request-shape" />
      ) : (
        !body.description && (
          <p className="mt-1 text-sm text-gray-500">
            Takes a body, but its fields are not described.
          </p>
        )
      )}
    </div>
  );
}

// ResponseSection is what comes back. A caller generating a client needs a
// model to decode into, and the alternative to being told is reading the
// browser's network tab and hard-coding whatever happened to be in the answer
// that day — including the fields that were empty and absent.
function ResponseSection({ entry, schemas }: { entry: Entry; schemas: Schemas }) {
  const schema = responseSchema(entry);

  return (
    <div>
      <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-500">
        Response
      </h3>
      {describesShape(schema, schemas) ? (
        <SchemaShape schema={schema} schemas={schemas} testId="response-shape" />
      ) : (
        <p className="mt-1 rounded border border-gray-200 bg-gray-50 px-3 py-2 text-xs text-gray-600">
          The shape of this answer is not described yet.
        </p>
      )}
    </div>
  );
}

function OperationPanel({
  entry,
  schemas,
  width,
  onClose,
}: {
  entry: Entry;
  schemas: Schemas;
  width: number;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const origin =
    typeof window === "undefined" ? "https://your-service" : window.location.origin;
  const curl = curlFor(entry, origin, schemas);
  const params = entry.op.parameters ?? [];
  const isWrite = !READ_METHODS.includes(entry.method);

  function handleCopy() {
    navigator.clipboard.writeText(curl).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <aside
      style={{ width }}
      // Bounded and scrolling itself, not just pinned. A sticky panel taller
      // than the window hangs its overflow below the fold, and the page's
      // scroll moves the list beside it rather than the panel — so the last
      // fields of a settings section with thirty of them cannot be reached at
      // all. The height allows for the padding of the scrolling region this
      // sits in.
      // The extra room at the foot is there to be seen: scrolled to the end,
      // text flush against the edge reads as more text below it, and the
      // reader keeps trying to scroll. It only applies where the panel
      // scrolls — in flow it is the card's own padding and needs no help.
      className="w-full shrink-0 space-y-4 rounded-md border border-gray-200 bg-white p-4 xl:sticky xl:top-0 xl:max-h-[calc(100vh-3rem)] xl:self-start xl:overflow-y-auto xl:pb-8">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span
              className={`rounded px-1.5 py-0.5 font-mono text-[11px] font-semibold uppercase ${
                METHOD_STYLE[entry.method] ?? "bg-gray-100 text-gray-700"
              }`}
            >
              {entry.method}
            </span>
            {entry.op["x-required-role"] && (
              <span
                className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${
                  ROLE_STYLE[entry.op["x-required-role"]] ??
                  "bg-gray-100 text-gray-600"
                }`}
              >
                {entry.op["x-required-role"]}
              </span>
            )}
          </div>
          <code className="mt-1.5 block break-all font-mono text-sm text-gray-900">
            {entry.path}
          </code>
        </div>
        <button
          onClick={onClose}
          aria-label="Close details"
          className="shrink-0 rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600"
        >
          ✕
        </button>
      </div>

      {entry.op.summary && (
        <p className="text-sm text-gray-600">{entry.op.summary}</p>
      )}

      {entry.op.operationId && (
        <p className="text-xs text-gray-500">
          Client method name:{" "}
          <code className="font-mono text-gray-700">
            {entry.op.operationId}
          </code>
        </p>
      )}

      <div>
        {/*
          Named for where these go rather than for what OpenAPI calls them, and
          paired with "Body parameters" below. A section headed "Parameters"
          saying "none", directly above a table of body fields, reads as a
          contradiction unless the reader already knows the word covers the
          address and the query string only — and the people this page is for
          are the ones who do not.
        */}
        <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-500">
          URI parameters
        </h3>
        {params.length === 0 ? (
          <p className="mt-1 text-sm text-gray-500">
            None — nothing in the address, and nothing accepted in the query
            string.
          </p>
        ) : (
          <table className="mt-1.5 w-full text-left text-sm">
            <thead>
              <tr className="text-xs uppercase tracking-wider text-gray-400">
                <th className="pb-1 font-medium">Name</th>
                <th className="pb-1 font-medium">In</th>
                <th className="pb-1 font-medium">Type</th>
              </tr>
            </thead>
            <tbody>
              {params.map((p) => (
                <tr key={`${p.in}-${p.name}`} className="border-t border-gray-100">
                  <td className="py-1 font-mono text-gray-900">
                    {p.name}
                    {p.required && <span className="text-red-600"> *</span>}
                  </td>
                  <td className="py-1 text-gray-600">{p.in}</td>
                  <td className="py-1 text-gray-600">
                    {p.schema?.type ?? "string"}
                    {limitsOf(p.schema) && (
                      <span className="text-gray-500"> {limitsOf(p.schema)}</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {isWrite && <RequestBodySection entry={entry} schemas={schemas} />}

      <ResponseSection entry={entry} schemas={schemas} />

      <div>
        <div className="flex items-center justify-between">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-500">
            Example
          </h3>
          <button
            onClick={handleCopy}
            className="rounded border border-gray-300 px-2 py-0.5 text-xs font-medium text-gray-700 hover:bg-gray-50"
          >
            {copied ? "Copied" : "Copy"}
          </button>
        </div>
        <pre
          data-testid="curl-example"
          className="mt-1.5 overflow-x-auto rounded bg-gray-900 p-3 font-mono text-xs leading-relaxed text-gray-100"
        >
          {curl}
        </pre>
        <p className="mt-1.5 text-xs text-gray-500">
          Set <code className="font-mono">APITOKEN</code> to a credential from
          your account page. It is a shell variable here on purpose — an example
          with a real one in it ends up pasted somewhere it should not be.
        </p>
      </div>
    </aside>
  );
}

export function ApiDocsPage() {
  const [doc, setDoc] = useState<OpenApiDocument | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  // Which groups the reader has opened. Closed is the starting state: there
  // are two hundred and forty-five addresses, and a page that opens as all of
  // them is one nobody reads to the end. A search overrides this — see below.
  const [opened, setOpened] = useState<ReadonlySet<string>>(new Set());

  // How wide the detail pane is, in pixels, remembered across visits. This is
  // the first thing in this app to use browser storage, so it is namespaced
  // and it degrades to the default rather than throwing.
  const [panelWidth, setPanelWidth] = useState(storedPanelWidth);
  const dragRef = useRef<{ startX: number; startWidth: number } | null>(null);

  // Dragging left widens the right-hand pane, so the delta is subtracted.
  // Measured as a delta from where the drag began rather than from the
  // container's edge, which keeps it correct inside any layout.
  const onSeparatorPointerDown = useCallback(
    (e: React.PointerEvent) => {
      dragRef.current = { startX: e.clientX, startWidth: panelWidth };
    },
    [panelWidth],
  );

  useEffect(() => {
    rememberPanelWidth(panelWidth);
  }, [panelWidth]);

  useEffect(() => {
    function move(e: PointerEvent) {
      const drag = dragRef.current;
      if (!drag) return;
      setPanelWidth(clampWidth(drag.startWidth - (e.clientX - drag.startX)));
    }
    function up() {
      dragRef.current = null;
    }
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    return () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
  }, []);

  // Keyboard is not a courtesy here: a pointer drag is unusable for anybody
  // who cannot make fine movements, and this is the only way to reach the
  // wide view of a long path.
  const onSeparatorKeyDown = useCallback((e: React.KeyboardEvent) => {
    const step = e.shiftKey ? 100 : 20;
    if (e.key === "ArrowLeft") {
      e.preventDefault();
      setPanelWidth((w) => clampWidth(w + step));
    } else if (e.key === "ArrowRight") {
      e.preventDefault();
      setPanelWidth((w) => clampWidth(w - step));
    } else if (e.key === "Home") {
      e.preventDefault();
      setPanelWidth(PANEL_MAX_WIDTH);
    } else if (e.key === "End") {
      e.preventDefault();
      setPanelWidth(PANEL_MIN_WIDTH);
    }
  }, []);

  const load = useCallback(() => {
    setLoading(true);
    setLoadError(null);
    fetchApiDocument()
      .then(setDoc)
      .catch((err: unknown) =>
        setLoadError(
          err instanceof Error
            ? err.message
            : "Failed to load the API description.",
        ),
      )
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => load(), [load]);

  const entries = useMemo(() => entriesOf(doc), [doc]);

  // Match the summary as well as the address: somebody who knew the address
  // would not have opened this page.
  const matched = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return entries;
    return entries.filter(
      (e) =>
        e.path.toLowerCase().includes(q) ||
        (e.op.summary ?? "").toLowerCase().includes(q) ||
        (e.op.operationId ?? "").toLowerCase().includes(q),
    );
  }, [entries, query]);

  const grouped = useMemo(() => {
    const map = new Map<string, Entry[]>();
    for (const e of matched) {
      const g = groupOf(e.path);
      const list = map.get(g);
      if (list) list.push(e);
      else map.set(g, [e]);
    }
    return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  }, [matched]);

  const selectedEntry = useMemo(
    () => entries.find((e) => `${e.method} ${e.path}` === selected) ?? null,
    [entries, selected],
  );

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-xl font-bold text-gray-800">
          {doc?.info?.title ?? "API"} API
        </h2>
        {doc?.info?.description && (
          <p className="mt-1 text-sm text-gray-600">{doc.info.description}</p>
        )}
        <p className="mt-1 text-xs text-gray-500">
          {/* The version is load-bearing: a description is only true of the
              build that served it. */}
          Generated from this build
          {doc?.info?.version ? ` — version ${doc.info.version}` : ""}. Machine
          readable at{" "}
          <a
            className="font-mono text-blue-600 hover:underline"
            href="/api/v1/openapi.json"
          >
            /api/v1/openapi.json
          </a>
          .
        </p>
      </div>

      {loading && <LoadingSpinner message="Loading the API description…" />}

      {/* A failure to load is reported as one. An empty list would say this
          service serves nothing. */}
      {!loading && loadError && (
        <ErrorAlert message={loadError} onRetry={load} />
      )}

      {!loading && !loadError && (
        <div className="flex flex-col gap-4 xl:flex-row xl:items-start">
          <div className="min-w-0 flex-1 space-y-4">
            <div className="flex items-baseline gap-3">
              <input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Filter by address or by what it is for…"
                aria-label="Filter the API description"
                className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
              <span className="shrink-0 text-xs text-gray-500">
                {matched.length} of {entries.length}
              </span>
            </div>

            {matched.length === 0 && (
              <p className="rounded-md border border-gray-200 bg-white p-4 text-sm text-gray-500">
                No addresses match that.
              </p>
            )}

            {grouped.map(([group, groupEntries]) => {
              // A search opens what it matched. A filter that hides its own
              // results behind a click is worse than no filter: the reader
              // types, sees nothing, and concludes the address is not there.
              const isOpen = query.trim() !== "" || opened.has(group);
              return (
              <section
                key={group}
                className="overflow-hidden rounded-md border border-gray-200 bg-white"
              >
                <h3>
                  <button
                    type="button"
                    aria-expanded={isOpen}
                    aria-controls={`group-${group}`}
                    onClick={() =>
                      setOpened((open) => {
                        const next = new Set(open);
                        if (next.has(group)) next.delete(group);
                        else next.add(group);
                        return next;
                      })
                    }
                    className="flex w-full items-center gap-2 border-b border-gray-200 bg-gray-50 px-4 py-2 text-left text-xs font-semibold uppercase tracking-wider text-gray-500 hover:bg-gray-100"
                  >
                    <span
                      aria-hidden="true"
                      className={`text-gray-400 transition-transform ${isOpen ? "rotate-90" : ""}`}
                    >
                      &#9656;
                    </span>
                    {group} ({groupEntries.length})
                  </button>
                </h3>
                {isOpen && (
                <ul id={`group-${group}`} className="divide-y divide-gray-100">
                  {groupEntries.map((e) => {
                    const key = `${e.method} ${e.path}`;
                    return (
                      <li key={key}>
                        <button
                          onClick={() =>
                            setSelected((s) => (s === key ? null : key))
                          }
                          // Spelled out rather than left to the computed name:
                          // nested inline elements concatenate without spaces,
                          // so the computed one reads "get/api/v1/cookbooks".
                          aria-label={`${e.method.toUpperCase()} ${e.path}`}
                          className={`flex w-full items-start gap-3 px-4 py-2.5 text-left hover:bg-gray-50 ${
                            selected === key ? "bg-blue-50" : ""
                          }`}
                        >
                          <span
                            className={`mt-0.5 w-16 shrink-0 rounded px-1.5 py-0.5 text-center font-mono text-[11px] font-semibold uppercase ${
                              METHOD_STYLE[e.method] ??
                              "bg-gray-100 text-gray-700"
                            }`}
                          >
                            {e.method}
                          </span>
                          <span className="min-w-0 flex-1">
                            <code className="break-all font-mono text-sm text-gray-900">
                              {e.path}
                            </code>
                            {e.op.summary && (
                              <span className="mt-0.5 block text-sm text-gray-600">
                                {e.op.summary}
                              </span>
                            )}
                          </span>
                          {e.op["x-required-role"] && (
                            <span
                              className={`mt-0.5 shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ${
                                ROLE_STYLE[e.op["x-required-role"]] ??
                                "bg-gray-100 text-gray-600"
                              }`}
                              title="The access this operation needs"
                            >
                              {e.op["x-required-role"]}
                            </span>
                          )}
                        </button>
                      </li>
                    );
                  })}
                </ul>
                )}
              </section>
              );
            })}
          </div>

          {selectedEntry && (
            <>
              <div
                role="separator"
                aria-orientation="vertical"
                aria-label="Resize the detail pane"
                aria-valuenow={panelWidth}
                aria-valuemin={PANEL_MIN_WIDTH}
                aria-valuemax={PANEL_MAX_WIDTH}
                tabIndex={0}
                onPointerDown={onSeparatorPointerDown}
                onKeyDown={onSeparatorKeyDown}
                title="Drag, or focus and use the arrow keys"
                className="hidden w-1.5 shrink-0 cursor-col-resize self-stretch rounded bg-gray-200 hover:bg-blue-400 focus:bg-blue-500 focus:outline-none xl:block"
              />
              <OperationPanel
                entry={selectedEntry}
                schemas={doc?.components?.schemas ?? {}}
                width={panelWidth}
                onClose={() => setSelected(null)}
              />
            </>
          )}
        </div>
      )}
    </div>
  );
}
