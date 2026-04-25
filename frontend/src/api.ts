// SPDX-License-Identifier: Apache-2.0
// Barrel re-export — all API functions now live in api/ domain files.
// This file exists for backward compatibility so existing imports
// from "./api" or "../api" continue to work unchanged.

export * from "./api/index";

// Re-export Pagination type from types for backward compatibility
// (the original api.ts re-exported it).
export type { Pagination } from "./types";
