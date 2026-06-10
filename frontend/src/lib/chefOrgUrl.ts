// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Validation for the "Chef Server URL" field. Operators paste the full
// organisation URL (https://<host>/organizations/<org>); the org name is
// derived from it server-side, so the URL must include the
// "/organizations/<org>" path segment.
//
// Returns a human-readable error message, or null when the URL is valid.
export function chefOrgURLError(url: string): string | null {
  const trimmed = url.trim();
  if (trimmed === "") return "Chef Server URL is required.";

  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    return "Enter a valid URL, e.g. https://chef.example.com/organizations/myorg";
  }

  if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
    return "URL must start with https:// (or http://).";
  }
  if (!/\/organizations\/[^/]+/.test(parsed.pathname)) {
    return "Must be a full organisation URL, e.g. https://chef.example.com/organizations/myorg";
  }
  return null;
}
