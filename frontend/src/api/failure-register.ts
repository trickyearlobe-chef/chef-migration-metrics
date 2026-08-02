// SPDX-License-Identifier: Apache-2.0

import type {
  FailureRegisterEntry,
  FailureRegisterResponse,
  RecordFailureVerdictBody,
  ReviseFailureEntryBody,
} from "../types";
import { apiFetch, buildUrl } from "./client";

/**
 * The standup view. Defaults to what is standing rather than the whole
 * history — pass status "all" for everything ever recorded.
 */
export function fetchFailureRegister(params?: {
  status?: string;
  verdict?: string;
  subject_name?: string;
  window_days?: number;
  page?: number;
  per_page?: number;
}): Promise<FailureRegisterResponse> {
  return apiFetch<FailureRegisterResponse>(
    buildUrl("/failure-register", params),
  );
}

/**
 * Records a person's verdict about a repo. A second verdict on a repo that
 * already has one is a reversal: the first is superseded, not overwritten.
 */
export function recordFailureVerdict(
  body: RecordFailureVerdictBody,
): Promise<FailureRegisterEntry> {
  return apiFetch<FailureRegisterEntry>(buildUrl("/failure-register"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

/**
 * Updates what is known and planned. The verdict and the reason cannot be
 * edited — record a new verdict instead.
 */
export function reviseFailureEntry(
  id: string,
  body: ReviseFailureEntryBody,
): Promise<FailureRegisterEntry> {
  return apiFetch<FailureRegisterEntry>(
    buildUrl(`/failure-register/${encodeURIComponent(id)}`),
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}

/** Records that a standing verdict has been dealt with. The entry stays. */
export function resolveFailureEntry(
  id: string,
  note?: string,
): Promise<FailureRegisterEntry> {
  return apiFetch<FailureRegisterEntry>(
    buildUrl(`/failure-register/${encodeURIComponent(id)}/resolve`),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ note: note ?? "" }),
    },
  );
}

/**
 * Every verdict ever recorded about one subject — where a reader sees that a
 * scan called it incompatible, a person overruled it, and why.
 */
export function fetchFailureRegisterHistory(
  subjectName: string,
): Promise<{ data: FailureRegisterEntry[] }> {
  return apiFetch<{ data: FailureRegisterEntry[] }>(
    // A repo name may contain a slash on some hosting platforms, and the API
    // takes the rest of the path whole, so it must not be escaped away.
    buildUrl(`/failure-register/subject/${subjectName}`),
  );
}
