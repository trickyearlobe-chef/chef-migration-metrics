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
import { MaintenanceProvider } from "./context/MaintenanceContext";
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
import { RunEventsPage } from "./pages/RunEventsPage";
import { RunEventNodeDetailPage } from "./pages/RunEventNodeDetailPage";
import { GitRepoDetailPage } from "./pages/GitRepoDetailPage";
import { GitRepoRemediationPage } from "./pages/GitRepoRemediationPage";
import { RemediationPage } from "./pages/RemediationPage";
import { LogsPage } from "./pages/LogsPage";
import { OwnersPage } from "./pages/OwnersPage";
import { OwnerDetailPage } from "./pages/OwnerDetailPage";
import { OwnershipAuditLogPage } from "./pages/OwnershipAuditLogPage";
import { OwnershipImportPage } from "./pages/OwnershipImportPage";
import { AdminUsersPage } from "./pages/AdminUsersPage";
import { AdminSystemHealthPage } from "./pages/AdminSystemHealthPage";
import { AdminCredentialsPage } from "./pages/credentials";
import { AdminTestKitchenHubPage } from "./pages/AdminTestKitchenHubPage";
import { AdminCookstylePage } from "./pages/AdminCookstylePage";
import { AdminGitURLsPage } from "./pages/AdminGitURLsPage";
import { AdminCollectionPage } from "./pages/AdminCollectionPage";
import { AdminLoggingPage } from "./pages/AdminLoggingPage";
import { AdminExportsPage } from "./pages/AdminExportsPage";
import { AdminReadinessPage } from "./pages/AdminReadinessPage";
import { AdminTargetVersionsPage } from "./pages/AdminTargetVersionsPage";
import { AdminOrganisationsPage } from "./pages/AdminOrganisationsPage";
import { AdminServerPage } from "./pages/AdminServerPage";
import { AdminAuthPage } from "./pages/AdminAuthPage";
import {
  AdminSetupWizardPage,
  useSetupRequired,
} from "./pages/AdminSetupWizardPage";
import { AdminPlatformDisplayNamesPage } from "./pages/AdminPlatformDisplayNamesPage";
import { AdminBackupPage } from "./pages/AdminBackupPage";
import { OwnerAliasesPage } from "./pages/OwnerAliasesPage";
import { OwnerDuplicatesPage } from "./pages/OwnerDuplicatesPage";
import { FailureRegisterPage } from "./pages/FailureRegisterPage";

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
// ---------------------------------------------------------------------------

// Routes that stay reachable while setup is incomplete. The wizard itself is
// here to avoid an infinite redirect loop; the credentials page is here
// because the organisation step requires a Chef API key credential, and the
// wizard directs the admin there to create one. Without this the guard would
// bounce the credentials page back to the wizard and setup would deadlock.
const SETUP_ALLOWED_PREFIXES = ["/admin/setup", "/admin/credentials"];

export function isSetupAllowedPath(pathname: string): boolean {
  return SETUP_ALLOWED_PREFIXES.some((prefix) => pathname.startsWith(prefix));
}

function SetupModeGuard({ children }: { children: React.ReactNode }) {
  const { isAdmin } = useAuth();
  const { setupRequired, checking } = useSetupRequired();
  const location = useLocation();

  // Only redirect admins; non-admins can't reach admin routes anyway.
  if (
    isAdmin &&
    !checking &&
    setupRequired &&
    !isSetupAllowedPath(location.pathname)
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
      <MaintenanceProvider>
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
              <Route path="/run-events" element={<RunEventsPage />} />
              <Route
                path="/run-events/nodes/:organisation/:node"
                element={<RunEventNodeDetailPage />}
              />
              <Route path="/remediation" element={<RemediationPage />} />
              <Route
                path="/failure-register"
                element={<FailureRegisterPage />}
              />
              <Route path="/ownership" element={<OwnersPage />} />
              <Route
                path="/ownership/aliases"
                element={<OwnerAliasesPage />}
              />
              <Route
                path="/ownership/audit-log"
                element={<OwnershipAuditLogPage />}
              />
              {/* Import and duplicate resolution live under /admin — they are
                  administrator functions, and the path says so. See the
                  admin-only routes below for the guards. */}
              <Route path="/ownership/:name" element={<OwnerDetailPage />} />
              <Route path="/logs" element={<LogsPage />} />

              {/* Admin-only routes */}
              <Route
                path="/admin/ownership/import"
                element={
                  <RequireAdmin>
                    <OwnershipImportPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/ownership/duplicates"
                element={
                  <RequireAdmin>
                    <OwnerDuplicatesPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/users"
                element={
                  <RequireAdmin>
                    <AdminUsersPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/system-stats"
                element={
                  <RequireAdmin>
                    <AdminSystemHealthPage />
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
                    <AdminTestKitchenHubPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/cookstyle"
                element={
                  <RequireAdmin>
                    <AdminCookstylePage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/kitchen-batches"
                element={<Navigate to="/admin/test-kitchen?tab=batches" replace />}
              />
              <Route
                path="/admin/kitchen-queue"
                element={<Navigate to="/admin/test-kitchen?tab=queue" replace />}
              />
              <Route
                path="/admin/kitchen-analysis"
                element={<Navigate to="/admin/test-kitchen?tab=analysis" replace />}
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
                element={<Navigate to="/admin/test-kitchen?tab=settings" replace />}
              />
              <Route
                path="/admin/config/analysis-tools"
                element={<Navigate to="/admin/cookstyle" replace />}
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
                path="/admin/config/readiness"
                element={
                  <RequireAdmin>
                    <AdminReadinessPage />
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
                path="/admin/config/platform-display-names"
                element={
                  <RequireAdmin>
                    <AdminPlatformDisplayNamesPage />
                  </RequireAdmin>
                }
              />
              <Route
                path="/admin/performance"
                element={<Navigate to="/admin/system-stats?tab=performance" replace />}
              />
              <Route
                path="/admin/backups"
                element={
                  <RequireAdmin>
                    <AdminBackupPage />
                  </RequireAdmin>
                }
              />
            </Route>

            {/* Catch-all — redirect to dashboard */}
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </AuthProvider>
      </BrowserRouter>
      </MaintenanceProvider>
    </ErrorBoundary>
  );
}
