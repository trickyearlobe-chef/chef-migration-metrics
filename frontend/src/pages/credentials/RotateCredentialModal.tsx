import { useState, type FormEvent } from "react";
import { updateCredential, ApiError } from "../../api";
import type { Credential } from "../../types";
import { typeLabel } from "./constants";
import { Modal } from "./Modal";
import { ValueField } from "./ValueField";

export function RotateCredentialModal({
  open,
  onClose,
  onRotated,
  target,
}: {
  open: boolean;
  onClose: () => void;
  onRotated: () => void;
  target: Credential | null;
}) {
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  if (!target) return null;

  function handleClose() {
    setValue("");
    setError(null);
    setSaving(false);
    onClose();
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await updateCredential(target!.name, { value });
      setValue("");
      setError(null);
      setSaving(false);
      onRotated();
      onClose();
    } catch (err: unknown) {
      const message =
        err instanceof ApiError ? err.message : "Failed to rotate credential";
      setError(message);
      setSaving(false);
    }
  }

  return (
    <Modal open={open} onClose={handleClose} title="Rotate Credential">
      <form onSubmit={handleSubmit} className="space-y-4">
        <p className="text-sm text-gray-600">
          Rotating{" "}
          <span className="font-medium text-gray-900">{target.name}</span> (
          {typeLabel(target.credential_type)})
        </p>

        {error && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </div>
        )}

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">
            New Value
          </label>
          <ValueField
            credentialType={target.credential_type}
            value={value}
            onChange={setValue}
            disabled={saving}
          />
        </div>

        <div className="flex items-center justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={handleClose}
            disabled={saving}
            className="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={saving}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:opacity-50"
          >
            {saving ? "Rotating\u2026" : "Rotate"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
