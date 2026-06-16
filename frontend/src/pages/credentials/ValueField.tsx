import { INPUT_CLS } from "./constants";

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
      placeholder={
        credentialType === "chef_client_key"
          ? "-----BEGIN RSA PRIVATE KEY-----\n..."
          : "Enter value\u2026"
      }
      value={value}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
      required
    />
  );
}
