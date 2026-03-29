import { useState, type FormEvent } from "react";
import { createCredential, ApiError } from "../../api";
import { CREDENTIAL_TYPES, INPUT_CLS } from "./constants";
import { Modal } from "./Modal";
import { ValueField } from "./ValueField";

export function CreateCredentialModal({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [credType, setCredType] = useState<string>(CREDENTIAL_TYPES[0].value);
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  function reset() {
    setName("");
    setCredType(CREDENTIAL_TYPES[0].value);
    setValue("");
    setError(null);
    setSaving(false);
  }

  function handleClose() {
    reset();
    onClose();
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await createCredential({
        name,
        credential_type: credType,
        value,
      });
      reset();
      onCreated();
      onClose();
    } catch (err: unknown) {
      const message =
        err instanceof ApiError ? err.message : "Failed to create credential";
      setError(message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal open={open} onClose={handleClose} title="Create Credential">
      <form onSubmit={handleSubmit} className="space-y-4">
        {error && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </div>
        )}

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">
            Name
          </label>
          <input
            type="text"
            className={INPUT_CLS}
            placeholder="e.g. vsphere-prod-key"
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={saving}
            required
          />
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">
            Type
          </label>
          <select
            className={INPUT_CLS}
            value={credType}
            onChange={(e) => setCredType(e.target.value)}
            disabled={saving}
          >
            {CREDENTIAL_TYPES.map((ct) => (
              <option key={ct.value} value={ct.value}>
                {ct.label}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">
            Value
          </label>
          <ValueField
            credentialType={credType}
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
            {saving ? "Creating\u2026" : "Create"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
