// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";
import { fetchFeatures } from "../api";
import type { Features } from "../api/features";

// Feature flags default OFF until loaded, so a gated surface never flashes
// visible before the flag resolves.
const DEFAULT: Features = { run_events: false };

// Module-level cache so every consumer (nav + node detail) shares one request.
let cache: Features | null = null;
let inflight: Promise<Features> | null = null;

// invalidateFeatures clears the cache so the next mount re-fetches — call after
// changing a feature flag (e.g. saving show_run_events) so the nav can update.
export function invalidateFeatures(): void {
  cache = null;
  inflight = null;
}

// useFeatures returns the viewer feature flags, fetched once and cached.
export function useFeatures(): Features {
  const [features, setFeatures] = useState<Features>(cache ?? DEFAULT);

  useEffect(() => {
    if (cache) return;
    if (!inflight) {
      // Wrapped so a throw / non-promise / rejection all fall back to DEFAULT
      // rather than crashing a consumer (or a test that mocks the api module).
      inflight = Promise.resolve()
        .then(() => fetchFeatures())
        .then((f) => f ?? DEFAULT)
        .catch(() => DEFAULT);
    }
    const pending = inflight;
    let active = true;
    pending.then((f) => {
      cache = f;
      if (active) setFeatures(f);
    });
    return () => {
      active = false;
    };
  }, []);

  return features;
}
