import { useCallback, useEffect, useState } from "react";
import {
  fetchNotifications,
  saveNotifications,
  type NotificationsConfig,
  type NotificationChannel,
} from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";
const SELECT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50 bg-white";

const ALL_EVENTS = [
  "cookbook_status_change",
  "readiness_milestone",
  "new_incompatible_cookbook",
  "collection_failure",
  "stale_node_threshold_exceeded",
  "certificate_expiry_warning",
] as const;

function SectionCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="border-b border-gray-100 px-4 py-3">
        <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
      </div>
      <div className="space-y-4 p-4">{children}</div>
    </div>
  );
}

function FieldRow({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-gray-700">{label}</label>
      {children}
      {hint && <p className="mt-1 text-xs text-gray-400">{hint}</p>}
    </div>
  );
}

function ChannelCard({
  channel,
  index,
  isOpen,
  saving,
  onToggle,
  onChange,
  onRemove,
}: {
  channel: NotificationChannel;
  index: number;
  isOpen: boolean;
  saving: boolean;
  onToggle: (index: number) => void;
  onChange: (index: number, updated: NotificationChannel) => void;
  onRemove: (index: number) => void;
}) {
  function setField<K extends keyof NotificationChannel>(field: K, value: NotificationChannel[K]) {
    onChange(index, { ...channel, [field]: value });
  }

  function toggleEvent(event: string) {
    const current = channel.events ?? [];
    const updated = current.includes(event)
      ? current.filter((e) => e !== event)
      : [...current, event];
    setField("events", updated);
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <button
        type="button"
        onClick={() => onToggle(index)}
        className="flex w-full items-center justify-between px-4 py-3 text-left hover:bg-gray-50"
      >
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-gray-900">
            {channel.name || "New Channel"}
          </span>
          <span
            className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
              channel.type === "webhook"
                ? "bg-purple-50 text-purple-700"
                : "bg-blue-50 text-blue-700"
            }`}
          >
            {channel.type}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onRemove(index); }}
            disabled={saving}
            title="Remove"
            className="shrink-0 rounded p-1.5 text-gray-400 hover:bg-red-50 hover:text-red-500 disabled:opacity-40"
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
            </svg>
          </button>
          <svg
            className={`h-4 w-4 text-gray-400 transition-transform ${isOpen ? "rotate-180" : ""}`}
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="m19.5 8.25-7.5 7.5-7.5-7.5" />
          </svg>
        </div>
      </button>

      {isOpen && (
        <div className="space-y-4 border-t border-gray-100 p-4">
          <div className="grid grid-cols-2 gap-4">
            <FieldRow label="Name">
              <input
                type="text"
                value={channel.name}
                onChange={(e) => setField("name", e.target.value)}
                placeholder="my-channel"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <FieldRow label="Type">
              <select
                value={channel.type}
                onChange={(e) => setField("type", e.target.value)}
                className={SELECT_CLASS}
                disabled={saving}
              >
                <option value="webhook">Webhook</option>
                <option value="email">Email</option>
              </select>
            </FieldRow>
          </div>

          {channel.type === "webhook" && (
            <div className="grid grid-cols-2 gap-4">
              <FieldRow label="URL" hint="Webhook endpoint URL">
                <input
                  type="url"
                  value={channel.url}
                  onChange={(e) => setField("url", e.target.value)}
                  placeholder="https://hooks.example.com/webhook"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
              <FieldRow label="URL Env" hint="Or environment variable containing the URL">
                <input
                  type="text"
                  value={channel.url_env}
                  onChange={(e) => setField("url_env", e.target.value)}
                  placeholder="WEBHOOK_URL"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
            </div>
          )}

          {channel.type === "email" && (
            <FieldRow label="Recipients" hint="Comma-separated email addresses">
              <input
                type="text"
                value={(channel.recipients ?? []).join(", ")}
                onChange={(e) =>
                  setField(
                    "recipients",
                    e.target.value.split(",").map((r) => r.trim()).filter(Boolean)
                  )
                }
                placeholder="admin@example.com, ops@example.com"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
          )}

          <div>
            <p className="mb-2 text-xs font-medium text-gray-700">Events</p>
            <div className="grid grid-cols-2 gap-x-4 gap-y-2">
              {ALL_EVENTS.map((event) => (
                <label key={event} className="flex cursor-pointer items-center gap-2">
                  <input
                    type="checkbox"
                    checked={(channel.events ?? []).includes(event)}
                    onChange={() => toggleEvent(event)}
                    disabled={saving}
                    className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 disabled:opacity-50"
                  />
                  <span className="text-xs text-gray-700">{event.replace(/_/g, " ")}</span>
                </label>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export function AdminNotificationsPage() {
  const [config, setConfig] = useState<NotificationsConfig | null>(null);
  const [saved, setSaved] = useState<NotificationsConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [openChannels, setOpenChannels] = useState<Set<number>>(new Set());
  const [newMilestone, setNewMilestone] = useState("");
  const [milestoneError, setMilestoneError] = useState<string | null>(null);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchNotifications()
      .then((data) => {
        if (cancelled) return;
        setConfig(data);
        setSaved(data);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(err instanceof Error ? err.message : "Failed to load notifications config.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => load(), [load]);

  const isDirty = JSON.stringify(config) !== JSON.stringify(saved);

  function setTopField<K extends keyof NotificationsConfig>(field: K, value: NotificationsConfig[K]) {
    setConfig((prev) => prev ? { ...prev, [field]: value } : prev);
    setSuccess(false);
  }

  function handleAddMilestone() {
    const n = parseInt(newMilestone, 10);
    if (isNaN(n) || n < 0 || n > 100) {
      setMilestoneError("Must be an integer between 0 and 100.");
      return;
    }
    setMilestoneError(null);
    const current = config?.readiness_milestones ?? [];
    if (!current.includes(n)) {
      setTopField("readiness_milestones", [...current, n].sort((a, b) => a - b));
    }
    setNewMilestone("");
  }

  function handleRemoveMilestone(value: number) {
    setTopField(
      "readiness_milestones",
      (config?.readiness_milestones ?? []).filter((m) => m !== value)
    );
  }

  function handleChannelToggle(index: number) {
    setOpenChannels((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  }

  function handleChannelChange(index: number, updated: NotificationChannel) {
    setConfig((prev) => {
      if (!prev) return prev;
      return { ...prev, channels: prev.channels.map((c, i) => i === index ? updated : c) };
    });
    setSuccess(false);
  }

  function handleChannelRemove(index: number) {
    setConfig((prev) => {
      if (!prev) return prev;
      return { ...prev, channels: prev.channels.filter((_, i) => i !== index) };
    });
    setOpenChannels((prev) => {
      const next = new Set<number>();
      prev.forEach((idx) => { if (idx !== index) next.add(idx > index ? idx - 1 : idx); });
      return next;
    });
    setSuccess(false);
  }

  function handleChannelAdd() {
    setConfig((prev) => {
      if (!prev) return prev;
      const newChannel: NotificationChannel = {
        name: "",
        type: "webhook",
        url: "",
        url_env: "",
        recipients: [],
        events: [],
        filters: { organisations: [], cookbooks: [] },
      };
      return { ...prev, channels: [...prev.channels, newChannel] };
    });
    setOpenChannels((prev) => {
      const next = new Set(prev);
      next.add((config?.channels.length ?? 0));
      return next;
    });
    setSuccess(false);
  }

  async function handleSave() {
    if (!config) return;
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    try {
      const { value: updated } = await saveNotifications(config);
      setConfig(updated ?? config);
      setSaved(updated ?? config);
      setSuccess(true);
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : "Failed to save notifications config.");
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingSpinner message="Loading notifications config…" />;
  if (loadError)
    return <ErrorAlert message="Failed to load notifications config" detail={loadError} onRetry={load} />;
  if (!config) return null;

  const notifEnabled = config.enabled;

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Notifications</h2>
        <p className="mt-1 text-sm text-gray-500">
          Configure notification channels for webhook and email delivery.
        </p>
      </div>

      {/* Global Settings */}
      <SectionCard title="Global Settings">
        <label className="flex cursor-pointer items-center gap-3">
          <div
            className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-within:ring-2 focus-within:ring-blue-500 focus-within:ring-offset-2 ${notifEnabled ? "bg-blue-600" : "bg-gray-200"}`}
          >
            <span
              className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition-transform ${notifEnabled ? "translate-x-4" : "translate-x-0"}`}
            />
            <input
              type="checkbox"
              className="sr-only"
              checked={notifEnabled}
              onChange={(e) => setTopField("enabled", e.target.checked)}
              disabled={saving}
            />
          </div>
          <span className="text-sm text-gray-700">Notifications Enabled</span>
        </label>

        <div>
          <p className="mb-1 text-xs font-medium text-gray-700">Readiness Milestones</p>
          <div className="flex flex-wrap gap-2 mb-2">
            {(config.readiness_milestones ?? []).map((m) => (
              <span
                key={m}
                className="inline-flex items-center gap-1 rounded-full bg-green-50 px-2.5 py-0.5 text-xs font-medium text-green-700"
              >
                {m}%
                <button
                  type="button"
                  onClick={() => handleRemoveMilestone(m)}
                  disabled={saving}
                  className="ml-0.5 text-green-400 hover:text-green-600 disabled:opacity-40"
                >
                  ×
                </button>
              </span>
            ))}
            {(config.readiness_milestones ?? []).length === 0 && (
              <span className="text-xs text-gray-400">No milestones set.</span>
            )}
          </div>
          <div className="flex items-start gap-2">
            <div className="flex-1">
              <input
                type="number"
                min={0}
                max={100}
                value={newMilestone}
                onChange={(e) => { setNewMilestone(e.target.value); setMilestoneError(null); }}
                onKeyDown={(e) => e.key === "Enter" && handleAddMilestone()}
                placeholder="e.g. 25"
                className={INPUT_CLASS}
                disabled={saving}
              />
              {milestoneError && <p className="mt-1 text-xs text-red-500">{milestoneError}</p>}
            </div>
            <button
              type="button"
              onClick={handleAddMilestone}
              disabled={saving || !newMilestone}
              className="shrink-0 rounded-md bg-gray-100 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200 disabled:opacity-40"
            >
              Add
            </button>
          </div>
        </div>

        <FieldRow
          label="Stale Node Alert Count"
          hint="Alert when stale node count exceeds this threshold (0 = disabled)"
        >
          <input
            type="number"
            min={0}
            value={config.stale_node_alert_count}
            onChange={(e) => setTopField("stale_node_alert_count", Number(e.target.value))}
            className={INPUT_CLASS}
            disabled={saving}
          />
        </FieldRow>
      </SectionCard>

      {/* Channels */}
      <SectionCard title="Channels">
        {config.channels.length === 0 ? (
          <p className="text-center text-sm text-gray-400 py-4">
            No channels configured. Add one below.
          </p>
        ) : (
          <div className="space-y-3">
            {config.channels.map((channel, i) => (
              <ChannelCard
                key={i}
                channel={channel}
                index={i}
                isOpen={openChannels.has(i)}
                saving={saving}
                onToggle={handleChannelToggle}
                onChange={handleChannelChange}
                onRemove={handleChannelRemove}
              />
            ))}
          </div>
        )}
        <button
          type="button"
          onClick={handleChannelAdd}
          disabled={saving}
          className="flex items-center gap-1.5 text-sm font-medium text-blue-600 hover:text-blue-700 disabled:opacity-40"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
          Add Channel
        </button>
      </SectionCard>

      {saveError && <ErrorAlert message="Failed to save" detail={saveError} />}

      {success && (
        <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">
          Settings saved successfully.
        </div>
      )}

      <div className="flex justify-end">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving || !isDirty}
          className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50"
        >
          {saving && <InlineSpinner />}
          {saving ? "Saving…" : "Save"}
        </button>
      </div>
    </div>
  );
}
