// SPDX-License-Identifier: Apache-2.0

import type { Pagination } from "./common";

export interface CookstyleViolationItem {
  source: "server" | "git";
  name: string;
  version: string;
  organisation?: string;
  target_chef_version: string;
  passed: boolean;
  offence_count: number;
  deprecation_count: number;
  correctness_count: number;
  error_message?: string;
  scanned_at: string;
  namespace_counts: Record<string, number>;
  severity_counts: Record<string, number>;
  top_cops: string[];
}

export interface CookstyleViolationsResponse {
  data: CookstyleViolationItem[];
  pagination: Pagination;
}
