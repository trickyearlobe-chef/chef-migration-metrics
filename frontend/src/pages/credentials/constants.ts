export const CREDENTIAL_TYPES = [
  { value: "chef_client_key", label: "Chef Client Key (PEM)" },
  // A connection string for an ownership import source. Its own type because it
  // has a shape worth checking when it is stored — it must name a database and
  // a driver we can open — so the refusal reaches whoever composed it rather
  // than the administrator who pastes it into an import weeks later.
  { value: "database_url", label: "Database Connection" },
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
  database_url: "bg-emerald-100 text-emerald-700",
  generic: "bg-gray-100 text-gray-700",
};

export const BADGE_LABELS: Record<string, string> = {
  chef_client_key: "Chef Key",
  database_url: "Database",
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
