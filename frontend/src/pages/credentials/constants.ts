export const CREDENTIAL_TYPES = [
  { value: "chef_client_key", label: "Chef Client Key (PEM)" },
  { value: "generic", label: "Generic" },
] as const;

export function typeLabel(t: string): string {
  const found = CREDENTIAL_TYPES.find((ct) => ct.value === t);
  return found ? found.label : t;
}

export const INPUT_CLS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500";

export const BADGE_STYLES: Record<string, string> = {
  chef_client_key: "bg-blue-100 text-blue-700",
  generic: "bg-gray-100 text-gray-700",
};

export const BADGE_LABELS: Record<string, string> = {
  chef_client_key: "Chef Key",
  generic: "Generic",
};

export function formatDate(dateStr?: string | null): string {
  if (!dateStr) return "\u2014";
  try {
    return new Date(dateStr).toLocaleString();
  } catch {
    return dateStr;
  }
}
