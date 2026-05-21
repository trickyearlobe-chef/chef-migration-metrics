// SPDX-License-Identifier: Apache-2.0

import { useMemo } from "react";
import cronstrue from "cronstrue";

interface CronDescriptionProps {
  expression: string;
}

export function CronDescription({ expression }: CronDescriptionProps) {
  const description = useMemo(() => {
    if (!expression.trim()) return null;
    try {
      return cronstrue.toString(expression, { use24HourTimeFormat: true });
    } catch {
      return null;
    }
  }, [expression]);

  if (!description) return null;

  return (
    <p className="mt-1 text-xs text-indigo-600">
      ↳ {description}
    </p>
  );
}
