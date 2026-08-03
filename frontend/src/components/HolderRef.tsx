// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// ---------------------------------------------------------------------------
// Who or what is holding a failure register entry: an owner's name, or a
// reference to a ticket in a system CMM does not read.
//
// The field is free text on purpose, and people paste ServiceNow and Jira links
// into it. A link you have to select and copy is a link nobody follows, so a
// pasted address is rendered as one.
//
// Only http and https are linked. The register is free text that any operator
// can write and everyone else reads, so a `javascript:` or `data:` address
// rendered as a link would be a script one colleague runs in another's session.
// Anything that is not a plain web address stays as text, which is also the
// right answer for a bare ticket number.
// ---------------------------------------------------------------------------

interface HolderRefProps {
  value: string;
  className?: string;
}

/** The link's text: the last meaningful part of the address, because a whole
 *  URL pushes a narrow column about and the tail is what a person recognises. */
function linkLabel(url: URL): string {
  const segments = url.pathname.split("/").filter(Boolean);
  const last = segments[segments.length - 1];
  if (last) return decodeURIComponent(last);
  // No path to speak of — the host is more use than an empty string.
  return url.host;
}

/** The address if it is a plain web link, otherwise null. */
function webAddress(value: string): URL | null {
  const trimmed = value.trim();
  // Without a scheme this is a ticket number, not an address. Guessing one
  // would turn "INC0012345" into a link to nowhere.
  if (!/^https?:\/\//i.test(trimmed)) return null;
  try {
    const url = new URL(trimmed);
    return url.protocol === "http:" || url.protocol === "https:" ? url : null;
  } catch {
    return null;
  }
}

export function HolderRef({ value, className }: HolderRefProps) {
  if (!value.trim()) return null;

  const url = webAddress(value);
  if (!url) {
    return <span className={className}>{value}</span>;
  }

  return (
    <a
      href={url.toString()}
      target="_blank"
      rel="noopener noreferrer"
      title={value}
      className={`text-blue-600 hover:text-blue-800 hover:underline ${className ?? ""}`}
    >
      {linkLabel(url)}
    </a>
  );
}
