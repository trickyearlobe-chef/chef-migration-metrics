import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { AuthProvider, useAuth } from "./context/AuthContext";
import { OrgProvider } from "./context/OrgContext";
import { AppLayout } from "./components/AppLayout";
import { LoginPage } from "./pages/LoginPage";
import { DashboardPage } from "./pages/DashboardPage";
import { NodesPage } from "./pages/NodesPage";
import { NodeDetailPage } from "./pages/NodeDetailPage";
import { CookbooksPage } from "./pages/CookbooksPage";
import { CookbookDetailPage } from "./pages/CookbookDetailPage";
import { CookbookCommittersPage } from "./pages/CookbookCommittersPage";
import { CookbookRemediationPage } from "./pages/CookbookRemediationPage";
import { RemediationPage } from "./pages/RemediationPage";
import { DependencyGraphPage } from "./pages/DependencyGraphPage";
import { LogsPage } from "./pages/LogsPage";
import { OwnersPage } from "./pages/OwnersPage";
import { OwnerDetailPage } from "./pages/OwnerDetailPage";
import { OwnershipAuditLogPage } from "./pages/OwnershipAuditLogPage";
import { OwnershipImportPage } from "./pages/OwnershipImportPage";
import { AdminUsersPage } from "./pages/AdminUsersPage";

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
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
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
// Login route — redirects to dashboard if already authenticated.
// ---------------------------------------------------------------------------

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
                  <AppLayout />
                </OrgProvider>
              </RequireAuth>
            }
          >
            <Route path="/" element={<DashboardPage />} />
            <Route path="/nodes" element={<NodesPage />} />
            <Route path="/nodes/:org/:name" element={<NodeDetailPage />} />
            <Route path="/cookbooks" element={<CookbooksPage />} />
            <Route path="/cookbooks/:name" element={<CookbookDetailPage />} />
            <Route path="/cookbooks/:name/committers" element={<CookbookCommittersPage />} />
            <Route
              path="/cookbooks/:name/:version/remediation"
              element={<CookbookRemediationPage />}
            />
            <Route path="/remediation" element={<RemediationPage />} />
            <Route path="/ownership" element={<OwnersPage />} />
            <Route path="/ownership/audit-log" element={<OwnershipAuditLogPage />} />
            <Route path="/ownership/import" element={<OwnershipImportPage />} />
            <Route path="/ownership/:name" element={<OwnerDetailPage />} />
            <Route path="/dependencies" element={<DependencyGraphPage />} />
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
          </Route>

          {/* Catch-all — redirect to dashboard */}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  );
}
