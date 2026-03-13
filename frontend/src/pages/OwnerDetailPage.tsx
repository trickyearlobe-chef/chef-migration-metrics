import { useState, useEffect, useCallback } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { useAuth } from "../context/AuthContext";
import {
  fetchOwnerDetail,
  fetchAssignments,
  createAssignments,
  deleteAssignment,
  updateOwner,
  deleteOwner,
  type AssignmentFilterQuery,
} from "../api";
import type {
  OwnerDetail,
  OwnershipAssignment,
  Pagination as PaginationType,
} from "../types";

// ---------------------------------------------------------------------------
// Display-label maps
// ---------------------------------------------------------------------------

const ENTITY_TYPE_LABELS: Record<string, string> = {
  node: "Node",
  cookbook: "Cookbook",
  git_repo: "Git Repo",
  role: "Role",
  policy: "Policy",
};

const SOURCE_LABELS: Record<string, string> = {
  manual: "Manual",
  auto_rule: "Auto Rule",
  import: "Import",
};

const CONFIDENCE_LABELS: Record<string, string> = {
  definitive: "Definitive",
  inferred: "Inferred",
};

const OWNER_TYPE_OPTIONS = [
  "team",
  "individual",
  "business_unit",
  "cost_centre",
  "custom",
] as const;

const ENTITY_TYPE_OPTIONS = [
  "node",
  "cookbook",
  "git_repo",
  "role",
  "policy",
] as const;

const PER_PAGE = 25;

// ---------------------------------------------------------------------------
// Owner detail page
// ---------------------------------------------------------------------------

export function OwnerDetailPage() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { user } = useAuth();

  const canEdit =
    user !== null && (user.role === "admin" || user.role === "operator");

  // -- Owner detail state ---------------------------------------------------
  const [owner, setOwner] = useState<OwnerDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // -- Assignments state ----------------------------------------------------
  const [assignments, setAssignments] = useState<OwnershipAssignment[]>([]);
  const [assignmentsPagination, setAssignmentsPagination] =
    useState<PaginationType | null>(null);
  const [assignmentsPage, setAssignmentsPage] = useState(1);
  const [entityTypeFilter, setEntityTypeFilter] = useState<string>("");
  const [assignmentsLoading, setAssignmentsLoading] = useState(false);
  const [assignmentsError, setAssignmentsError] = useState<string | null>(null);

  // -- Add assignment form --------------------------------------------------
  const [showAddForm, setShowAddForm] = useState(false);
  const [newEntityType, setNewEntityType] = useState<string>("node");
  const [newEntityKey, setNewEntityKey] = useState("");
  const [newNotes, setNewNotes] = useState("");
  const [addLoading, setAddLoading] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);

  // -- Edit modal state -----------------------------------------------------
  const [showEditModal, setShowEditModal] = useState(false);
  const [editDisplayName, setEditDisplayName] = useState("");
  const [editContactEmail, setEditContactEmail] = useState("");
  const [editContactChannel, setEditContactChannel] = useState("");
  const [editOwnerType, setEditOwnerType] = useState("");
  const [editLoading, setEditLoading] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);

  // -- Delete state ---------------------------------------------------------
  const [deleteLoading, setDeleteLoading] = useState(false);

  // -- Load owner detail ----------------------------------------------------
  const loadOwner = useCallback(() => {
    if (!name) return;
    setLoading(true);
    setError(null);
    fetchOwnerDetail(name)
      .then(setOwner)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [name]);

  useEffect(() => {
    loadOwner();
  }, [loadOwner]);

  // -- Load assignments -----------------------------------------------------
  const loadAssignments = useCallback(() => {
    if (!name) return;
    setAssignmentsLoading(true);
    setAssignmentsError(null);
    const filters: AssignmentFilterQuery = {
      page: assignmentsPage,
      per_page: PER_PAGE,
    };
    if (entityTypeFilter) {
      filters.entity_type = entityTypeFilter;
    }
    fetchAssignments(name, filters)
      .then((res) => {
        setAssignments(res.data);
        setAssignmentsPagination(res.pagination);
      })
      .catch((e: Error) => setAssignmentsError(e.message))
      .finally(() => setAssignmentsLoading(false));
  }, [name, assignmentsPage, entityTypeFilter]);

  useEffect(() => {
    loadAssignments();
  }, [loadAssignments]);

  // -- Handlers -------------------------------------------------------------

  const handleEntityTypeFilterChange = (val: string) => {
    setEntityTypeFilter(val);
    setAssignmentsPage(1);
  };

  const handleAddAssignment = async () => {
    if (!name || !newEntityKey.trim()) return;
    setAddLoading(true);
    setAddError(null);
    try {
      await createAssignments(name, {
        assignments: [
          {
            entity_type: newEntityType,
            entity_key: newEntityKey.trim(),
            notes: newNotes.trim() || undefined,
          },
        ],
      });
      setShowAddForm(false);
      setNewEntityType("node");
      setNewEntityKey("");
      setNewNotes("");
      loadAssignments();
    } catch (e: unknown) {
      setAddError(e instanceof Error ? e.message : "Failed to create assignment");
    } finally {
      setAddLoading(false);
    }
  };

  const handleDeleteAssignment = async (assignmentId: string) => {
    if (!name) return;
    if (!window.confirm("Delete this assignment?")) return;
    try {
      await deleteAssignment(name, assignmentId);
      loadAssignments();
    } catch (e: unknown) {
      window.alert(
        e instanceof Error ? e.message : "Failed to delete assignment",
      );
    }
  };

  const openEditModal = () => {
    if (!owner) return;
    setEditDisplayName(owner.display_name ?? "");
    setEditContactEmail(owner.contact_email ?? "");
    setEditContactChannel(owner.contact_channel ?? "");
    setEditOwnerType(owner.owner_type);
    setEditError(null);
    setShowEditModal(true);
  };

  const handleEditSave = async () => {
    if (!name) return;
    setEditLoading(true);
    setEditError(null);
    try {
      const updated = await updateOwner(name, {
        display_name: editDisplayName || undefined,
        contact_email: editContactEmail || undefined,
        contact_channel: editContactChannel || undefined,
        owner_type: editOwnerType || undefined,
      });
      // Merge updated fields back into the existing detail (keep summaries).
      setOwner((prev) => (prev ? { ...prev, ...updated } : prev));
      setShowEditModal(false);
    } catch (e: unknown) {
      setEditError(e instanceof Error ? e.message : "Failed to update owner");
    } finally {
      setEditLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!name) return;
    if (
      !window.confirm(
        `Are you sure you want to delete owner "${name}"? This cannot be undone.`,
      )
    )
      return;
    setDeleteLoading(true);
    try {
      await deleteOwner(name);
      navigate("/ownership");
    } catch (e: unknown) {
      window.alert(e instanceof Error ? e.message : "Failed to delete owner");
      setDeleteLoading(false);
    }
  };

  // -- Early returns --------------------------------------------------------

  if (loading) return <LoadingSpinner message="Loading owner detail…" />;
  if (error) return <ErrorAlert message={error} onRetry={loadOwner} />;
  if (!owner) return null;

  // -- Derived values -------------------------------------------------------

  const totalPages = assignmentsPagination?.total_pages ?? 1;

  // -- Render ---------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <nav className="text-sm text-gray-500">
        <Link
          to="/ownership"
          className="hover:text-blue-600 hover:underline"
        >
          Ownership
        </Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">{owner.name}</span>
      </nav>

      {/* Header */}
      <div className="flex items-center gap-3">
        <h2 className="text-xl font-bold text-gray-800">{owner.name}</h2>
        <span className="inline-flex items-center rounded-full bg-blue-100 px-2.5 py-0.5 text-xs font-medium text-blue-700">
          {owner.owner_type}
        </span>
        {canEdit && (
          <div className="ml-auto flex items-center gap-2">
            <button
              type="button"
              onClick={openEditModal}
              className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700"
            >
              Edit
            </button>
            <button
              type="button"
              onClick={handleDelete}
              disabled={deleteLoading}
              className="rounded-md bg-red-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50"
            >
              {deleteLoading ? "Deleting…" : "Delete"}
            </button>
          </div>
        )}
      </div>

      {/* Info grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <InfoCard
          label="Display Name"
          value={owner.display_name || "—"}
        />
        <InfoCard
          label="Contact Email"
          value={owner.contact_email || "—"}
        />
        <InfoCard
          label="Contact Channel"
          value={owner.contact_channel || "—"}
        />
        <InfoCard label="Owner Type" value={owner.owner_type} />
        <InfoCard
          label="Created"
          value={new Date(owner.created_at).toLocaleString()}
        />
        <InfoCard
          label="Updated"
          value={new Date(owner.updated_at).toLocaleString()}
        />
      </div>

      {/* Summary cards */}
      {owner.readiness_summary && (
        <div className="card">
          <h3 className="card-header">Readiness Summary</h3>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <InfoCard
              label="Target Chef Version"
              value={owner.readiness_summary.target_chef_version}
            />
            <InfoCard
              label="Total Nodes"
              value={String(owner.readiness_summary.total_nodes)}
            />
            <InfoCard
              label="Ready Nodes"
              value={String(owner.readiness_summary.ready_nodes)}
            />
            <InfoCard
              label="Blocked Nodes"
              value={String(owner.readiness_summary.blocked_nodes)}
            />
          </div>
          {owner.readiness_summary.blocking_cookbooks.length > 0 && (
            <div className="mt-3">
              <span className="text-xs font-medium text-gray-500">
                Blocking Cookbooks:
              </span>
              <div className="mt-1 flex flex-wrap gap-1">
                {owner.readiness_summary.blocking_cookbooks.map((cb) => (
                  <Link
                    key={cb}
                    to={`/cookbooks/${encodeURIComponent(cb)}`}
                    className="rounded bg-red-50 px-1.5 py-0.5 text-xs text-red-700 hover:bg-red-100"
                  >
                    {cb}
                  </Link>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {owner.cookbook_summary && (
        <div className="card">
          <h3 className="card-header">Cookbook Summary</h3>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <InfoCard
              label="Total Cookbooks"
              value={String(owner.cookbook_summary.total_cookbooks)}
            />
            <InfoCard
              label="Compatible"
              value={String(owner.cookbook_summary.compatible)}
            />
            <InfoCard
              label="Incompatible"
              value={String(owner.cookbook_summary.incompatible)}
            />
            <InfoCard
              label="Untested"
              value={String(owner.cookbook_summary.untested)}
            />
          </div>
        </div>
      )}

      {owner.git_repo_summary && (
        <div className="card">
          <h3 className="card-header">Git Repo Summary</h3>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <InfoCard
              label="Total Repos"
              value={String(owner.git_repo_summary.total_repos)}
            />
            <InfoCard
              label="Compatible"
              value={String(owner.git_repo_summary.compatible)}
            />
            <InfoCard
              label="Incompatible"
              value={String(owner.git_repo_summary.incompatible)}
            />
          </div>
        </div>
      )}

      {/* Assignments section */}
      <div className="card">
        <div className="flex items-center justify-between">
          <h3 className="card-header">Assignments</h3>
          <div className="flex items-center gap-3">
            {/* Entity type filter */}
            <select
              value={entityTypeFilter}
              onChange={(e) => handleEntityTypeFilterChange(e.target.value)}
              className="rounded-md border border-gray-300 px-2 py-1 text-xs text-gray-700 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            >
              <option value="">All Types</option>
              {ENTITY_TYPE_OPTIONS.map((t) => (
                <option key={t} value={t}>
                  {ENTITY_TYPE_LABELS[t]}
                </option>
              ))}
            </select>
            {canEdit && (
              <button
                type="button"
                onClick={() => {
                  setShowAddForm((prev) => !prev);
                  setAddError(null);
                }}
                className="rounded-md bg-green-600 px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-green-700"
              >
                {showAddForm ? "Cancel" : "Add Assignment"}
              </button>
            )}
          </div>
        </div>

        {/* Inline add form */}
        {showAddForm && (
          <div className="mt-4 rounded-lg border border-gray-200 bg-gray-50 p-4">
            <div className="grid gap-3 sm:grid-cols-3">
              <div>
                <label className="mb-1 block text-xs font-medium text-gray-600">
                  Entity Type
                </label>
                <select
                  value={newEntityType}
                  onChange={(e) => setNewEntityType(e.target.value)}
                  className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                >
                  {ENTITY_TYPE_OPTIONS.map((t) => (
                    <option key={t} value={t}>
                      {ENTITY_TYPE_LABELS[t]}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-gray-600">
                  Entity Key
                </label>
                <input
                  type="text"
                  value={newEntityKey}
                  onChange={(e) => setNewEntityKey(e.target.value)}
                  placeholder="e.g. my-cookbook"
                  className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-gray-600">
                  Notes
                </label>
                <input
                  type="text"
                  value={newNotes}
                  onChange={(e) => setNewNotes(e.target.value)}
                  placeholder="Optional notes"
                  className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
            </div>
            {addError && (
              <p className="mt-2 text-xs text-red-600">{addError}</p>
            )}
            <div className="mt-3 flex justify-end">
              <button
                type="button"
                onClick={handleAddAssignment}
                disabled={addLoading || !newEntityKey.trim()}
                className="rounded-md bg-blue-600 px-4 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
              >
                {addLoading ? "Creating…" : "Create"}
              </button>
            </div>
          </div>
        )}

        {/* Assignments table */}
        {assignmentsLoading ? (
          <LoadingSpinner message="Loading assignments…" />
        ) : assignmentsError ? (
          <ErrorAlert
            message={assignmentsError}
            onRetry={loadAssignments}
          />
        ) : assignments.length === 0 ? (
          <p className="py-8 text-center text-sm text-gray-400">
            No assignments found.
          </p>
        ) : (
          <>
            <div className="mt-4 overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-gray-200 text-xs font-medium uppercase tracking-wider text-gray-500">
                    <th className="px-3 py-2">Entity Type</th>
                    <th className="px-3 py-2">Entity Key</th>
                    <th className="px-3 py-2">Source</th>
                    <th className="px-3 py-2">Confidence</th>
                    <th className="px-3 py-2">Notes</th>
                    {canEdit && <th className="px-3 py-2">Actions</th>}
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {assignments.map((a) => (
                    <tr key={a.id} className="hover:bg-gray-50">
                      <td className="px-3 py-2 text-gray-700">
                        {ENTITY_TYPE_LABELS[a.entity_type] ?? a.entity_type}
                      </td>
                      <td className="px-3 py-2 font-medium text-gray-800">
                        {a.entity_key}
                      </td>
                      <td className="px-3 py-2 text-gray-600">
                        {SOURCE_LABELS[a.assignment_source] ??
                          a.assignment_source}
                      </td>
                      <td className="px-3 py-2 text-gray-600">
                        {CONFIDENCE_LABELS[a.confidence] ?? a.confidence}
                      </td>
                      <td className="px-3 py-2 text-gray-500">
                        {a.notes || "—"}
                      </td>
                      {canEdit && (
                        <td className="px-3 py-2">
                          <button
                            type="button"
                            onClick={() => handleDeleteAssignment(a.id)}
                            className="text-xs text-red-600 hover:text-red-800 hover:underline"
                          >
                            Delete
                          </button>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <div className="mt-4 flex items-center justify-between text-xs text-gray-500">
                <span>
                  Page {assignmentsPagination?.page ?? 1} of {totalPages}
                  {assignmentsPagination?.total_items != null && (
                    <> — {assignmentsPagination.total_items} total</>
                  )}
                </span>
                <div className="flex gap-2">
                  <button
                    type="button"
                    disabled={assignmentsPage <= 1}
                    onClick={() =>
                      setAssignmentsPage((p) => Math.max(1, p - 1))
                    }
                    className="rounded border border-gray-300 px-2 py-1 hover:bg-gray-100 disabled:opacity-40"
                  >
                    ← Prev
                  </button>
                  <button
                    type="button"
                    disabled={assignmentsPage >= totalPages}
                    onClick={() =>
                      setAssignmentsPage((p) =>
                        Math.min(totalPages, p + 1),
                      )
                    }
                    className="rounded border border-gray-300 px-2 py-1 hover:bg-gray-100 disabled:opacity-40"
                  >
                    Next →
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      {/* Edit modal */}
      {showEditModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
            <h3 className="text-lg font-bold text-gray-800">Edit Owner</h3>

            <div className="mt-4 space-y-3">
              <div>
                <label className="mb-1 block text-xs font-medium text-gray-600">
                  Display Name
                </label>
                <input
                  type="text"
                  value={editDisplayName}
                  onChange={(e) => setEditDisplayName(e.target.value)}
                  className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-gray-600">
                  Contact Email
                </label>
                <input
                  type="email"
                  value={editContactEmail}
                  onChange={(e) => setEditContactEmail(e.target.value)}
                  className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-gray-600">
                  Contact Channel
                </label>
                <input
                  type="text"
                  value={editContactChannel}
                  onChange={(e) => setEditContactChannel(e.target.value)}
                  placeholder="e.g. #team-infra"
                  className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-gray-600">
                  Owner Type
                </label>
                <select
                  value={editOwnerType}
                  onChange={(e) => setEditOwnerType(e.target.value)}
                  className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                >
                  {OWNER_TYPE_OPTIONS.map((t) => (
                    <option key={t} value={t}>
                      {t}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            {editError && (
              <p className="mt-3 text-xs text-red-600">{editError}</p>
            )}

            <div className="mt-5 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setShowEditModal(false)}
                disabled={editLoading}
                className="rounded-md border border-gray-300 px-4 py-2 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-100 disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleEditSave}
                disabled={editLoading}
                className="rounded-md bg-blue-600 px-4 py-2 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
              >
                {editLoading ? "Saving…" : "Save"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Helper components
// ---------------------------------------------------------------------------

function InfoCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="stat-card">
      <span className="stat-label">{label}</span>
      <span className="mt-1 text-sm font-medium text-gray-800">{value}</span>
    </div>
  );
}
