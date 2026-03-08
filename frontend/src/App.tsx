import { BrowserRouter, Routes, Route } from "react-router-dom";
import { OrgProvider } from "./context/OrgContext";
import { AppLayout } from "./components/AppLayout";
import { DashboardPage } from "./pages/DashboardPage";
import { NodesPage } from "./pages/NodesPage";
import { NodeDetailPage } from "./pages/NodeDetailPage";
import { CookbooksPage } from "./pages/CookbooksPage";
import { CookbookDetailPage } from "./pages/CookbookDetailPage";
import { CookbookRemediationPage } from "./pages/CookbookRemediationPage";
import { RemediationPage } from "./pages/RemediationPage";
import { DependencyGraphPage } from "./pages/DependencyGraphPage";
import { LogsPage } from "./pages/LogsPage";

export function App() {
  return (
    <BrowserRouter>
      <OrgProvider>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="/" element={<DashboardPage />} />
            <Route path="/nodes" element={<NodesPage />} />
            <Route path="/nodes/:org/:name" element={<NodeDetailPage />} />
            <Route path="/cookbooks" element={<CookbooksPage />} />
            <Route path="/cookbooks/:name" element={<CookbookDetailPage />} />
            <Route path="/cookbooks/:name/:version/remediation" element={<CookbookRemediationPage />} />
            <Route path="/remediation" element={<RemediationPage />} />
            <Route path="/dependencies" element={<DependencyGraphPage />} />
            <Route path="/logs" element={<LogsPage />} />
          </Route>
        </Routes>
      </OrgProvider>
    </BrowserRouter>
  );
}
