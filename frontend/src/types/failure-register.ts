// SPDX-License-Identifier: Apache-2.0

import type { Pagination } from "./common";

/**
 * The failure register — a person's verdict on whether a cookbook actually
 * works on the target version, recorded with a reason.
 *
 * Two-sided on purpose. The automated signals are wrong in both directions:
 * CookStyle marks cookbooks blocked that demonstrably run fine, and Test
 * Kitchen reports the test environment falling over as a cookbook that does
 * not work.
 */
export type FailureVerdict = "broken" | "not_broken";

export type FailureStatus = "open" | "resolved" | "superseded";

/** A commitment holder is a person we know about, a user, or a reference to
 * work tracked in another system. CMM holds the reference and does not read
 * the system behind it. */
export type HolderType = "owner" | "user" | "ticket";

export interface FailureRegisterEntry {
  id: string;
  /** The subject: the repo is where a fix is made and re-released. */
  git_repo_name: string;
  /** The label: standup says "cookbook" while looking at repo-level work. */
  cookbook_name: string;
  verdict: FailureVerdict;
  reason: string;
  evidence?: string;
  diagnosis?: string;
  plan?: string;
  /** YYYY-MM-DD, where one has been given. */
  target_date?: string;
  holder_type?: HolderType;
  holder_ref?: string;
  status: FailureStatus;
  raised_by: string;
  raised_at: string;
  updated_at: string;
  resolved_by?: string;
  resolved_at?: string;
  resolution_note?: string;
  /** The reversal that replaced this verdict, so the disagreement stays
   * readable rather than being overwritten. */
  superseded_by?: string;
}

/**
 * How large the register is and which way it is moving. The size and the
 * direction matter as much as the contents: a register that is growing is a
 * different message from one that is shrinking.
 */
export interface FailureRegisterSummary {
  open: number;
  open_broken: number;
  open_not_broken: number;
  open_without_holder: number;
  open_overdue: number;

  window_days: number;
  raised_in_window: number;
  resolved_in_window: number;

  total_broken: number;
  total_not_broken: number;
  resolved: number;
}

export interface FailureRegisterResponse {
  data: FailureRegisterEntry[];
  pagination: Pagination;
  /** Absent when the summary failed to load — the list is still worth having. */
  summary?: FailureRegisterSummary;
}

export interface RecordFailureVerdictBody {
  git_repo_name: string;
  cookbook_name: string;
  verdict: FailureVerdict;
  reason: string;
  evidence?: string;
  diagnosis?: string;
  plan?: string;
  target_date?: string;
  holder_type?: HolderType;
  holder_ref?: string;
}

/** The verdict and the reason are deliberately absent: a reversal is a new
 * verdict, so the original stays readable. */
export interface ReviseFailureEntryBody {
  diagnosis?: string;
  plan?: string;
  evidence?: string;
  target_date?: string;
  holder_type?: HolderType;
  holder_ref?: string;
}
