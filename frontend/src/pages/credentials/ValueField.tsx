import { INPUT_CLS } from "./constants";

// A database connection is no longer typed here. It is configuration an
// administrator reads and edits on the import screen, and the credential beside
// it holds the password on its own — see journeys/ownership-connection.md.
function placeholderFor(credentialType: string): string {
  switch (credentialType) {
    case "chef_client_key":
      return "-----BEGIN RSA PRIVATE KEY-----\n...";
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
