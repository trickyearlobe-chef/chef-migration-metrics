// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// What the web interface actually sends, per address, resolved with the
// TypeScript compiler rather than read off the screen.
//
// Reading the source is reasoning; this is measurement. It is how three
// breakages were found before they broke anything: a reclassification sending
// a target version nothing read, a settings box for a value this service had
// never had, and the server settings screen handing back the read-only status
// it had just been shown. None of them would have been caught by the interface
// test suite — 31 of the 45 page test files mock the API module outright, and
// nothing there drives a real body into a real handler.
//
// Run from the frontend directory:
//   node ../tools/frontend-fields/record.cjs > \
//     ../internal/webapi/testdata/frontend_request_fields.json
//
// TestFrontend_EverythingTheInterfaceSendsIsAFieldWeRead holds the result
// against the served description.

const path = require('path');
const ts = require(path.join(process.cwd(), 'node_modules', 'typescript'));

const cfgPath = ts.findConfigFile('.', ts.sys.fileExists, 'tsconfig.json');
const parsed = ts.parseJsonConfigFileContent(
  ts.readConfigFile(cfgPath, ts.sys.readFile).config, ts.sys, path.dirname(cfgPath));
const program = ts.createProgram(parsed.fileNames, parsed.options);
const checker = program.getTypeChecker();

// propsOf walks unions and intersections so an optional-heavy payload type
// still reports every field it could carry.
function propsOf(type) {
  const names = new Set();
  const seen = new Set();
  (function walk(t, depth) {
    if (!t || depth > 3) return;
    if (t.isUnion && t.isUnion()) return t.types.forEach((x) => walk(x, depth));
    if (t.isIntersection && t.isIntersection()) return t.types.forEach((x) => walk(x, depth));
    const id = checker.typeToString(t);
    if (seen.has(id)) return;
    seen.add(id);
    for (const p of checker.getPropertiesOfType(t)) names.add(p.getName());
  })(type, 0);
  // Symbol-keyed members of array types, which are not fields on the wire.
  return [...names].filter((n) => !n.startsWith('__')).sort();
}

function enclosingCall(node, name) {
  let n = node;
  while (n && !(ts.isCallExpression(n) && new RegExp(name).test(n.expression.getText()))) n = n.parent;
  return n;
}

function methodOf(node) {
  let n = node;
  while (n && !ts.isObjectLiteralExpression(n)) n = n.parent;
  if (!n) return null;
  const m = n.properties.find((p) => p.name && p.name.getText() === 'method');
  return m && m.initializer ? m.initializer.getText().replace(/["']/g, '') : null;
}

// address turns the URL as written into the shape the API description uses:
// every named segment becomes {p}, so a template literal with an id in it
// lines up with "/api/v1/owners/{name}".
function address(text) {
  let u = text.trim();
  const built = u.match(/^buildUrl\(\s*([\s\S]*?)\s*\)$/);
  if (built) u = built[1];
  u = u.split(',')[0].trim().replace(/^[`"']|[`"']$/g, '');
  // A module-level path constant, resolved before the named segments are, or
  // `${PATH}/${id}` would come out as "{p}/{p}" and match nothing.
  u = u.replace(/\$\{PATH\}/g, '/saved-filters');
  if (u === 'PATH') u = '/saved-filters';
  u = u.replace(/\$\{[^}]*\}/g, '{p}');
  if (!u.startsWith('/api/v1')) u = '/api/v1' + u;
  return u;
}

const out = {};

for (const sf of program.getSourceFiles()) {
  if (sf.isDeclarationFile || sf.fileName.includes('.test.')) continue;
  if (!sf.fileName.includes('/src/')) continue;

  ts.forEachChild(sf, function visit(node) {
    if (ts.isCallExpression(node)) {
      // The ordinary shape: apiFetch(url, {method, body: JSON.stringify(x)}).
      if (node.expression.getText() === 'JSON.stringify' && node.arguments.length) {
        const call = enclosingCall(node, 'apiFetch');
        const method = methodOf(node);
        if (call && method && call.arguments.length) {
          const type = checker.getTypeAtLocation(node.arguments[0]);
          const name = checker.typeToString(type);
          // A JSON array body has no field names, and the generic settings
          // helper is resolved through its callers below.
          if (!name.endsWith('[]') && name !== 'unknown' && name !== 'any') {
            out[`${method} ${address(call.arguments[0].getText())}`] = propsOf(type);
          }
        }
      }
      // The settings screens all go through one helper, so the site itself
      // says `unknown`. Resolve the callers instead.
      if (/apiMutateConfig/.test(node.expression.getText()) && node.arguments.length >= 2) {
        const type = checker.getTypeAtLocation(node.arguments[1]);
        if (!checker.typeToString(type).endsWith('[]')) {
          out[`PUT ${address(node.arguments[0].getText())}`] = propsOf(type);
        }
      }
    }
    ts.forEachChild(node, visit);
  });
}

const sorted = {};
for (const key of Object.keys(out).sort()) sorted[key] = out[key];
console.log(JSON.stringify(sorted, null, 1));
