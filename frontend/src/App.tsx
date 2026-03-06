import { BrowserRouter, Routes, Route } from "react-router-dom";
import { OrgProvider } from "./context/OrgContext";
import { AppLayout } from "./components/AppLayout";
import { DashboardPage } from "./pages/DashboardPage";
import { NodesPage } from "./pages/NodesPage";
import { NodeDetailPage } from "./pages/NodeDetailPage";
import { CookbooksPage } from "./pages/CookbooksPage";
import { CookbookDetailPage } from "./pages/CookbookDetailPage";

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
          </Route>
        </Routes>
      </OrgProvider>
    </BrowserRouter>
  );
}
