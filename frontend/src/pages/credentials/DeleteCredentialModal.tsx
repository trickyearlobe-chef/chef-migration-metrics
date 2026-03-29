import { useState } from "react";
import { deleteCredential, ApiError } from "../../api";
import type { Credential } from "../../types";
import { Modal } from "./Modal";

export function DeleteCredentialModal({
  open,
  onClose,
  onDeleted,
  target,
}: {
  open: boolean;
  onClose: () => void;
  onDeleted: () => void;
  target: Credential | null;
}) {
  const [error, setError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  if (!target) return null;

  function handleClose() {
    setError(null);
    setDeleting(false);
    onClose();
  }

  async function handleConfirm() {
    setDeleting(true);
    setError(null);
    try {
      await deleteCredential(target!.name);
      setError(null);
      setDeleting(false);
      onDeleted();
      onClose();
    } catch (err: unknown) {
      const message =
        err instanceof ApiError ? err.message : "Failed to delete credential";
      setError(message);
      setDeleting(false);
    }
  }

  return (
    <Modal open={open} onClose={handleClose} title="Delete Credential">
      <div className="space-y-4">
        {error && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </div>
        )}

        <p className="text-sm text-gray-600">
          Are you sure you want to delete{" "}
          <span className="font-medium text-gray-900">{target.name}</span>? This
          action cannot be undone.
        </p>

        <div className="flex items-center justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={handleClose}
            disabled={deleting}
            className="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleConfirm}
            disabled={deleting}
            className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-red-700 disabled:opacity-50"
          >
            {deleting ? "Deleting\u2026" : "Delete"}
          </button>
        </div>
      </div>
    </Modal>
  );
}
