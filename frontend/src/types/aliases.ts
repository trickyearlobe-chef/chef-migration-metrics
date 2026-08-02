// SPDX-License-Identifier: Apache-2.0

export interface OwnerAlias {
  id: string;
  owner_name: string;
  alias_type: string;
  alias_value: string;
  source: string;
  created_at: string;
}

export interface AliasSuggestion {
  owner_name: string;
  alias_value: string;
  alias_type: string;
  similarity: number;
}

export interface AliasImportResponse {
  imported: number;
  skipped: number;
  errors: { line: number; error: string }[] | null;
}

/** One pair of owners that may be the same person. A lead, never a match. */
export interface OwnerDuplicateCandidate {
  owner_a: string;
  owner_b: string;
  /** "name" when the owner names are similar, "alias" when two identities are. */
  matched_on: string;
  value_a: string;
  value_b: string;
  similarity: number;
  assignments_a: number;
  assignments_b: number;
}

export interface OwnerDuplicatesResponse {
  /** How many pairs have been rejected — an empty list worked down to nothing
   * means something different from one nobody has read. */
  dismissed_pairs?: number;
  data: OwnerDuplicateCandidate[];
  pagination: {
    page: number;
    per_page: number;
    total_items: number;
    total_pages: number;
  };
  /** Absent when the count could not be taken. */
  coverage: {
    owners_total?: number;
    owners_without_alias?: number;
  };
  /** Absent until the catalogue has been scanned at least once. */
  scan?: {
    scanned_at: string;
    pairs_found: number;
  };
  scan_running: boolean;
}

export interface OwnerDuplicateRescanResponse {
  started: boolean;
  reason?: string;
}

export interface MergeOwnersResult {
  from_owner: string;
  into_owner: string;
  reassigned: number;
  skipped: number;
  aliases_moved: number;
  aliases_dropped: number;
  source_name_aliased: boolean;
}

/** A pair somebody has recorded as different people. Listed separately because
 * a dismissed pair is hidden from the candidate list — without somewhere to see
 * it, there is nothing to click to undo one. */
export interface OwnerDuplicateDismissal {
  owner_a: string;
  owner_b: string;
  reason?: string;
  dismissed_by: string;
  dismissed_at: string;
}
