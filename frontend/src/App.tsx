import {
  BrowserRouter,
  Routes,
  Route,
  Navigate,
  useLocation,
} from "react-router-dom";
import { AuthProvider, useAuth } from "./context/AuthContext";
import { OrgProvider } from "./context/OrgContext";
import { GlobalFilterProvider } from "./context/GlobalFilterContext";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { AppLayout } from "./components/AppLayout";
import { LoginPage } from "./pages/LoginPage";
import { DashboardPage } from "./pages/dashboard";
import { NodesPage } from "./pages/NodesPage";
import { NodeDetailPage } from "./pages/NodeDetailPage";
import { NodeDiskDetailPage } from "./pages/NodeDiskDetailPage";
import { CookbooksPage } from "./pages/CookbooksPage";
import { CookbookDetailPage } from "./pages/CookbookDetailPage";
import { CookbookCommittersPage } from "./pages/CookbookCommittersPage";
import { CookbookRemediationPage } from "./pages/CookbookRemediationPage";
import { RolesPage } from "./pages/RolesPage";
import { RoleDetailPage } from "./pages/RoleDetailPage";
import { GitReposPage } from "./pages/GitReposPage";
import { GitRepoDetailPage } from "./pages/GitRepoDetailPage";
import { GitRepoRemediationPage } from "./pages/GitRepoRemediationPage";
import { RemediationPage } from "./pages/RemediationPage";
import { LogsPage } from "./pages/LogsPage";
import { OwnersPage } from "./pages/OwnersPage";
import { OwnerDetailPage } from "./pages/OwnerDetailPage";
import { OwnershipAuditLogPage } from "./pages/OwnershipAuditLogPage";
import { OwnershipImportPage } from "./pages/OwnershipImportPage";
import { AdminUsersPage } from "./pages/AdminUsersPage";
import { AdminActionsPage } from "./pages/AdminActionsPage";
import { AdminSystemStatsPage } from "./pages/AdminSystemStatsPage";
import { AdminPerformancePage } from "./pages/AdminPerformancePage";
import { AdminCredentialsPage } from "./pages/credentials";
import { AdminTestKitchenPage } from "./pages/AdminTestKitchenPage";
import KitchenBatchesPage from "./pages/KitchenBatchesPage";
import KitchenQueuePage from "./pages/KitchenQueuePage";
import { AdminKitchenAnalysisPage } from "./pages/AdminKitchenAnalysisPage";
import { AdminGitURLsPage } from "./pages/AdminGitURLsPage";
import { AdminCollectionPage } from "./pages/AdminCollectionPage";
import { AdminLoggingPage } from "./pages/AdminLoggingPage";
import { AdminConcurrencyPage } from "./pages/AdminConcurrencyPage";
import { AdminAnalysisToolsPage } from "./pages/AdminAnalysisToolsPage";
import { AdminExportsPage } from "./pages/AdminExportsPage";
import { AdminTargetVersionsPage } from "./pages/AdminTargetVersionsPage";
import { AdminOrganisationsPage } from "./pages/AdminOrganisationsPage";
import { AdminServerPage } from "./pages/AdminServerPage";
import { AdminAuthPage } from "./pages/AdminAuthPage";
import { AdminNotificationsPage } from "./pages/AdminNotificationsPage";
import {
  AdminSetupWizardPage,
  useSetupRequired,
} from "./pages/AdminSetupWizardPage";
import { AdminPlatformDisplayNamesPage } from "./pages/AdminPlatformDisplayNamesPage";
import { OwnerAliasesPage } from "./pages/OwnerAliasesPage";

// ---------------------------------------------------------------------------
// Route guard — redirects to /login when the user is not authenticated.
// ---------------------------------------------------------------------------

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading } = useAuth();

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="flex items-center gap-2 text-sm text-gray-500">
          <svg
            className="h-5 w-5 animate-spin text-blue-600"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              className="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              strokeWidth="4"
            />
            <path
              className="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
            />
          </svg>
          Loading\u2026
        </div>
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

// ---------------------------------------------------------------------------
// Admin route guard — redirects non-admin users to the dashboard.
// ---------------------------------------------------------------------------

function RequireAdmin({ children }: { children: React.ReactNode }) {
  const { isAdmin } = useAuth();

  if (!isAdmin) {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}

// ---------------------------------------------------------------------------
// Setup mode guard — admins with empty config_store are sent to /admin/setup.
// Skips on the setup page itself to avoid infinite redirect loops.
// ---------------------------------------------------------------------------

function SetupModeGuard({ children }: { children: React.ReactNode }) {
  const { isAdmin } = useAuth();
  const { setupRequired, checking } = useSetupRequired();
  const location = useLocation();

  // Only redirect admins; non-admins can't reach admin routes anyway.
  if (
    isAdmin &&
    !checking &&
    setupRequired &&
    !location.pathname.startsWith("/admin/setup")
  ) {
    return <Navigate to="/admin/setup" replace />;
  }

  return <>{children}</>;
}

function LoginRoute() {
  const { isAuthenticated, loading } = useAuth();

  if (loading) return null;
  if (isAuthenticated) return <Navigate to="/" replace />;
  return <LoginPage />;
}

// ---------------------------------------------------------------------------
// Application root
// ---------------------------------------------------------------------------

export function App() {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <AuthProvider>
          <Routes>
            {/* Public route: login */}
            <Route path="/login" element={<LoginRoute />} />

            {/* Protected routes */}
            <Route
              element={
                <RequireAuth>
                  <OrgProvider>
                    <GlobalFilterProvider>
                      <SetupModeGuard>
                        <AppLayout />
                      </SetupModeGuard>
                    </GlobalFilterProvider>
                  </OrgProvider>
                </RequireAuth>
              }
            >
              {/* Setup wizard — shown to admins when config_store has no organisations */}
              <Route
                path="/admin/setup"
                element={
                  <RequireAdmin>
                    <AdminSetupWizardPage />
                  </RequireAdmin>
                }
              />
              <Route path="/" element={<DashboardPage />} />
              <Route path="/nodes" element={<NodesPage />} />
              <Route path="/nodes/:org/:name" element={<NodeDetailPage />} />
              <Route
                path="/nodes/:org/:name/disks"
                element={<NodeDiskDetailPage />}
              />
              <Route path="/cookbooks" element={<CookbooksPage />} />
              <Route path="/cookbooks/:name" element={<CookbookDetailPage />} />
              <Route
                path="/cookbooks/:name/committers"
                element={<CookbookCommittersPage />}
              />
              <Route
                path="/cookbooks/:name/:version/remediation"
                element={<CookbookRemediationPage />}
              />
              <Route path="/roles" element={<RolesPage />} />
              <Route path="/roles/:name" element={<RoleDetailPage />} />
              <Route path="/git-repos" element={<GitReposPage />} />
              <Route path="/git-repos/:name" element={<GitRepoDetailPage />} />
              <Route
                path="/git-repos/:name/:version/remediation"
                element={<GitRepoRemediationPage />}
              />
              <Route path="/remediation" element={<RemediationPage />} />
              <Route path="/ownership" element={<OwnersPage />} />
              <Route
                path="/ownership/aliases"
                element={<OwnerAliasesPage />}
              />
              <Route
                path="/ownership/audit-log"
                element={<OwnershipAuditLogPage />}
              />
              <Route
                path="/ownership/import"
                element={<OwnershipImportPage />}
              />
              <Route path="/ownership/:name" element={<OwnerDetailPage />} />
              <Route path="/logs" element={<LogsPage />} />

              {/* Admin-only routes */}
              <Route
                path="/admin/users"
                element={
                  <RequireAdmin>
                    <AdminUsersPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/actions"
                element={
                  <RequireAdmin>
                    <AdminActionsPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/system-stats"
                element={
                  <RequireAdmin>
                    <AdminSystemStatsPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/credentials"
                element={
                  <RequireAdmin>
                    <AdminCredentialsPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/test-kitchen"
                element={
                  <RequireAdmin>
                    <AdminTestKitchenPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/kitchen-batches"
                element={
                  <RequireAdmin>
                    <KitchenBatchesPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/kitchen-queue"
                element={
                  <RequireAdmin>
                    <KitchenQueuePage />
                  </RequireAdmin>
                }
              />

              <Route
                path="/admin/kitchen-analysis"
                element={
                  <RequireAdmin>
                    <AdminKitchenAnalysisPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/git-urls"
                element={
                  <RequireAdmin>
                    <AdminGitURLsPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/collection"
                element={
                  <RequireAdmin>
                    <AdminCollectionPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/logging"
                element={
                  <RequireAdmin>
                    <AdminLoggingPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/concurrency"
                element={
                  <RequireAdmin>
                    <AdminConcurrencyPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/analysis-tools"
                element={
                  <RequireAdmin>
                    <AdminAnalysisToolsPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/exports"
                element={
                  <RequireAdmin>
                    <AdminExportsPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/target-versions"
                element={
                  <RequireAdmin>
                    <AdminTargetVersionsPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/organisations"
                element={
                  <RequireAdmin>
                    <AdminOrganisationsPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/server"
                element={
                  <RequireAdmin>
                    <AdminServerPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/auth"
                element={
                  <RequireAdmin>
                    <AdminAuthPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/notifications"
                element={
                  <RequireAdmin>
                    <AdminNotificationsPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/config/platform-display-names"
                element={
                  <RequireAdmin>
                    <AdminPlatformDisplayNamesPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/performance"
                element={
                  <RequireAdmin>
                    <AdminPerformancePage />
                  </RequireAdmin>
                }
              />
            </Route>

            {/* Catch-all — redirect to dashboard */}
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </AuthProvider>
      </BrowserRouter>
    </ErrorBoundary>
  );
}
