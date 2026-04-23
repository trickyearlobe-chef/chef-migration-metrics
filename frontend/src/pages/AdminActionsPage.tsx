import { useState, useCallback } from "react";
import { rescanAllCookstyle, rerunAllTestKitchen } from "../api";

export function AdminActionsPage() {
  const [rescanningAll, setRescanningAll] = useState(false);
  const [rescanAllMsg, setRescanAllMsg] = useState<string | null>(null);
  const [showRescanAllConfirm, setShowRescanAllConfirm] = useState(false);

  const [rerunningTK, setRerunningTK] = useState(false);
  const [rerunTKMsg, setRerunTKMsg] = useState<string | null>(null);
  const [showRerunTKConfirm, setShowRerunTKConfirm] = useState(false);

  const handleRescanAll = useCallback(() => {
    setRescanningAll(true);
    setRescanAllMsg(null);
    setShowRescanAllConfirm(false);
    rescanAllCookstyle()
      .then((res) => {
        setRescanAllMsg(res.message);
      })
      .catch((e: Error) => setRescanAllMsg(`Rescan all failed: ${e.message}`))
      .finally(() => setRescanningAll(false));
  }, []);

  const handleRerunAllTK = useCallback(() => {
    setRerunningTK(true);
    setRerunTKMsg(null);
    setShowRerunTKConfirm(false);
    rerunAllTestKitchen()
      .then((res) => {
        setRerunTKMsg(res.message);
      })
      .catch((e: Error) => setRerunTKMsg(`Rerun all failed: ${e.message}`))
      .finally(() => setRerunningTK(false));
  }, []);

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-bold text-gray-800">Admin Actions</h2>

      {/* CookStyle Rescan */}
      <div className="card">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h3 className="text-base font-semibold text-gray-800">Rescan All CookStyle</h3>
            <p className="mt-1 text-sm text-gray-500">
              Invalidate all cached CookStyle results, complexity scores, and autocorrect previews
              across every cookbook. A collection run will be triggered immediately to rescan all cookbooks.
            </p>
            <p className="mt-1 text-xs text-gray-400">
              This is useful after upgrading CookStyle, changing target Chef versions, or when
              scan results appear stale.
            </p>
          </div>
          <button
            onClick={() => setShowRescanAllConfirm(true)}
            disabled={rescanningAll}
            className="shrink-0 inline-flex items-center gap-1.5 rounded-md border border-purple-300 bg-white px-4 py-2 text-sm font-medium text-purple-700 shadow-sm hover:bg-purple-50 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {rescanningAll ? "Requesting…" : "Rescan All Cookbooks"}
          </button>
        </div>

        {showRescanAllConfirm && (
          <div className="mt-4 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
            <p className="font-medium">Are you sure?</p>
            <p className="mt-1 text-amber-600">
              This will delete all cached CookStyle results, complexity scores, and autocorrect
              previews, then trigger an immediate collection run to rescan everything. This may
              take a significant amount of time depending on the number of cookbooks.
            </p>
            <div className="mt-3 flex gap-2">
              <button
                onClick={handleRescanAll}
                className="rounded-md bg-purple-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-purple-700"
              >
                Yes, Rescan All
              </button>
              <button
                onClick={() => setShowRescanAllConfirm(false)}
                className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {rescanAllMsg && (
          <div className={`mt-4 rounded-md border px-4 py-3 text-sm ${rescanAllMsg.startsWith("Rescan all failed")
            ? "border-red-200 bg-red-50 text-red-800"
            : "border-green-200 bg-green-50 text-green-800"
            }`}>
            {rescanAllMsg}
          </div>
        )}
      </div>

      {/* Test Kitchen Rerun */}
      <div className="card">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h3 className="text-base font-semibold text-gray-800">Rerun All Test Kitchen</h3>
            <p className="mt-1 text-sm text-gray-500">
              Invalidate all cached Test Kitchen results across every git repo. A collection run
              will be triggered immediately to retest all cookbooks.
            </p>
            <p className="mt-1 text-xs text-gray-400">
              This is useful after updating the kitchen configuration, changing target Chef versions,
              or modifying driver settings.
            </p>
          </div>
          <button
            onClick={() => setShowRerunTKConfirm(true)}
            disabled={rerunningTK}
            className="shrink-0 inline-flex items-center gap-1.5 rounded-md border border-orange-300 bg-white px-4 py-2 text-sm font-medium text-orange-700 shadow-sm hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {rerunningTK ? "Requesting…" : "Rerun All Test Kitchen"}
          </button>
        </div>

        {showRerunTKConfirm && (
          <div className="mt-4 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
            <p className="font-medium">Are you sure?</p>
            <p className="mt-1 text-amber-600">
              This will delete all cached Test Kitchen results and trigger an immediate collection
              run to retest everything. This may take a significant amount of time depending on
              the number of cookbooks and target Chef versions.
            </p>
            <div className="mt-3 flex gap-2">
              <button
                onClick={handleRerunAllTK}
                className="rounded-md bg-orange-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-orange-700"
              >
                Yes, Rerun All
              </button>
              <button
                onClick={() => setShowRerunTKConfirm(false)}
                className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {rerunTKMsg && (
          <div className={`mt-4 rounded-md border px-4 py-3 text-sm ${rerunTKMsg.startsWith("Rerun all failed")
            ? "border-red-200 bg-red-50 text-red-800"
            : "border-green-200 bg-green-50 text-green-800"
            }`}>
            {rerunTKMsg}
          </div>
        )}
      </div>
    </div>
  );
}
