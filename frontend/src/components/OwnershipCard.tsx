// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { Link } from "react-router-dom";
import type { EntityOwnership } from "../types";

// ---------------------------------------------------------------------------
// Who owns this thing, on a detail page.
//
// One component for every entity, reading the ownership the API already sends
// with the record, so a detail page and a list cannot answer this differently.
//
// "Nobody owns this" is said out loud rather than left blank. A blank space
// reads as "not applicable" or "still loading", and the whole point of the
// ownership work is to make the gaps visible — an unowned thing is precisely
// what somebody needs to go and find an owner for.
// ---------------------------------------------------------------------------

interface OwnershipCardProps {
  ownership?: EntityOwnership;
  /** What the ownership was derived from, when it was: "git repo". */
  derivedFrom?: string;
  className?: string;
}

export function OwnershipCard({
  ownership,
  derivedFrom = "git repo",
  className,
}: OwnershipCardProps) {
  // Absent is not the same as empty: the API could not tell us, so saying
  // "nobody owns this" would be an assertion nobody has earned.
  if (!ownership) return null;

  const owners = ownership.owners ?? [];

  return (
    <div className={`card ${className ?? ""}`}>
      <h3 className="card-header">Ownership</h3>
      {owners.length === 0 ? (
        <p className="text-sm text-amber-700">
          Nobody owns this yet.{" "}
          <Link
            to="/ownership/import"
            className="text-blue-600 hover:text-blue-800 hover:underline"
          >
            Import ownership
          </Link>{" "}
          or assign somebody on the owner’s page.
        </p>
      ) : (
        <>
          <ul className="flex flex-wrap gap-2">
            {owners.map((name) => (
              <li key={name}>
                <Link
                  to={`/ownership/${encodeURIComponent(name)}`}
                  className="inline-flex items-center rounded-full bg-blue-100 px-2.5 py-0.5 text-sm text-blue-800 hover:bg-blue-200"
                >
                  {name}
                </Link>
              </li>
            ))}
          </ul>
          {ownership.derived && (
            // Say where it came from, rather than implying somebody assigned
            // it here. A reader who wants to change it needs to know that the
            // repo is where the assignment lives.
            <p className="mt-2 text-xs text-gray-500">
              Taken from the {derivedFrom} this is built from — that is where
              the assignment lives.
            </p>
          )}
        </>
      )}
    </div>
  );
}
