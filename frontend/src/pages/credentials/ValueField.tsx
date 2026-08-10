import { INPUT_CLS } from "./constants";

// The example matters more for a connection string than for a key: it is the
// only place the required shape is stated, and the value is refused without a
// database in it.
function placeholderFor(credentialType: string): string {
  switch (credentialType) {
    case "chef_client_key":
      return "-----BEGIN RSA PRIVATE KEY-----\n...";
    case "database_url":
      return "postgres://user:pass@host:5432/DATABASE\nsqlserver://user:pass@host:1433?database=DATABASE";
    default:
      return "Enter value\u2026";
  }
}

export function ValueField({
  credentialType,
  value,
  onChange,
  disabled,
}: {
  credentialType: string;
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
}) {
  return (
    <textarea
      className={`${INPUT_CLS} font-mono`}
      rows={credentialType === "chef_client_key" ? 6 : 3}
      placeholder={placeholderFor(credentialType)}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
      required
    />
  );
}
