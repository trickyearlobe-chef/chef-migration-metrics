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
