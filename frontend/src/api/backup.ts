// SPDX-License-Identifier: Apache-2.0

import { apiFetch, buildUrl } from "./client";
import type { PutConfigResponse } from "./config";

// ---------------------------------------------------------------------------
// Backup config (GET/PUT /admin/config/backup)
// ---------------------------------------------------------------------------

export interface BackupConfig {
  enabled: boolean;
  dir: string;
  max_generations: number;
  schedule: string;
  pg_dump_path: string;
  pg_restore_path: string;
}

export function fetchBackupConfig(): Promise<BackupConfig> {
  return apiFetch<BackupConfig>(buildUrl("/admin/config/backup"));
}

export function saveBackupConfig(
  value: BackupConfig,
): Promise<PutConfigResponse<BackupConfig>> {
  return apiFetch<{ value: BackupConfig; restart_required?: boolean }>(
    buildUrl("/admin/config/backup"),
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(value),
    },
  ).then((envelope) => ({
    value: envelope.value,
    restartRequired: envelope.restart_required ?? false,
  }));
}

// ---------------------------------------------------------------------------
// Backup operations (CRUD /admin/backups)
// ---------------------------------------------------------------------------

export interface BackupItem {
  id: string;
  filename: string;
  size_bytes: number;
  sha256?: string;
  status: "pending" | "running" | "succeeded" | "failed" | "deleting" | "restoring";
  error?: string;
  created_at: string;
  completed_at?: string;
  initiated_by?: string;
}

export interface BackupStatus {
  active: boolean;
  id?: string;
  status?: string;
}

export interface BackupListResponse {
  backups: BackupItem[];
  backup_dir: string;
}

export function fetchBackups(): Promise<BackupListResponse> {
  return apiFetch<BackupListResponse>(buildUrl("/admin/backups"));
}

export function fetchBackupStatus(): Promise<BackupStatus> {
  return apiFetch<BackupStatus>(buildUrl("/admin/backups/status"));
}

export function createBackup(): Promise<{ id: string; status: string }> {
  return apiFetch<{ id: string; status: string }>(buildUrl("/admin/backups"), {
    method: "POST",
  });
}

export function deleteBackup(id: string): Promise<void> {
  return apiFetch<void>(buildUrl(`/admin/backups/${id}`), {
    method: "DELETE",
  });
}

export function restoreBackup(id: string): Promise<{ id: string; status: string; message: string }> {
  return apiFetch<{ id: string; status: string; message: string }>(
    buildUrl(`/admin/backups/${id}/restore`),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ confirm: "RESTORE" }),
    },
  );
}
