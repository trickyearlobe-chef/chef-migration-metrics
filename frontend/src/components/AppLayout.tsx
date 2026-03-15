import { NavLink, Outlet } from "react-router-dom";
import { OrgSelector } from "./OrgSelector";
import { HealthBadge } from "./HealthBadge";
import { useAuth } from "../context/AuthContext";

const navItems = [
  { to: "/", label: "Dashboard", icon: "M3.75 3v11.25A2.25 2.25 0 006 16.5h2.25M3.75 3h-1.5m1.5 0h16.5m0 0h1.5m-1.5 0v11.25A2.25 2.25 0 0118 16.5h-2.25m-7.5 0h7.5m-7.5 0l-1 3m8.5-3 1 3m0 0 .5 1.5m-.5-1.5h-9.5m0 0-.5 1.5M9 11.25v1.5M12 9v3.75m3-6v6" },
  { to: "/nodes", label: "Nodes", icon: "M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 01-3 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008zm-3 6h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z" },
  { to: "/cookbooks", label: "Cookbooks", icon: "M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25" },
  { to: "/git-repos", label: "Git Repos", icon: "M17.25 6.75 22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3-4.5 16.5" },
  { to: "/dependencies", label: "Dependencies", icon: "M7.217 10.907a2.25 2.25 0 1 0 0 2.186m0-2.186c.18.324.283.696.283 1.093s-.103.77-.283 1.093m0-2.186 9.566-5.314m-9.566 7.5 9.566 5.314m0 0a2.25 2.25 0 1 0 3.935 2.186 2.25 2.25 0 0 0-3.935-2.186Zm0-12.814a2.25 2.25 0 1 0 3.933-2.185 2.25 2.25 0 0 0-3.933 2.185Z" },
  { to: "/remediation", label: "Remediation", icon: "M11.42 15.17 17.25 21A2.652 2.652 0 0 0 21 17.25l-5.877-5.877M11.42 15.17l2.496-3.03c.317-.384.74-.626 1.208-.766M11.42 15.17l-4.655 5.653a2.548 2.548 0 1 1-3.586-3.586l6.837-5.63m5.108-.233c.55-.164 1.163-.188 1.743-.14a4.5 4.5 0 0 0 4.486-6.336l-3.276 3.277a3.004 3.004 0 0 1-2.25-2.25l3.276-3.276a4.5 4.5 0 0 0-6.336 4.486c.049.58.025 1.193-.14 1.743" },
  { to: "/ownership", label: "Ownership", icon: "M18 18.72a9.094 9.094 0 0 0 3.741-.479 3 3 0 0 0-4.682-2.72m.94 3.198.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0 1 12 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 0 1 6 18.719m12 0a5.971 5.971 0 0 0-.941-3.197m0 0A5.995 5.995 0 0 0 12 12.75a5.995 5.995 0 0 0-5.058 2.772m0 0a3 3 0 0 0-4.681 2.72 8.986 8.986 0 0 0 3.74.477m.94-3.197a5.971 5.971 0 0 0-.94 3.197M15 6.75a3 3 0 1 1-6 0 3 3 0 0 1 6 0Zm6 3a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Zm-13.5 0a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Z" },
  { to: "/logs", label: "Logs", icon: "M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" },
];

// Admin-only nav items shown below a separator.
const adminNavItems = [
  { to: "/admin/users", label: "Users", icon: "M15 19.128a9.38 9.38 0 0 0 2.625.372 9.337 9.337 0 0 0 4.121-.952 4.125 4.125 0 0 0-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128H9m6 0a5.972 5.972 0 0 0-.786-3.07M9 19.128v-.003c0-1.113.285-2.16.786-3.07M9 19.128H3.375a1.125 1.125 0 0 1-1.125-1.125v-1.5c0-.621.504-1.125 1.125-1.125h3.026a5.972 5.972 0 0 1 .786-3.07M9 19.128a5.972 5.972 0 0 1-.786-3.07m0 0A5.974 5.974 0 0 1 12 12.75a5.974 5.974 0 0 1 3.786 3.308m-3.786-3.308a5.25 5.25 0 1 1 0-10.5 5.25 5.25 0 0 1 0 10.5" },
  { to: "/admin/actions", label: "Actions", icon: "M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" },
];

export function AppLayout() {
  const { user, isAdmin, logout } = useAuth();

  return (
    <div className="flex h-screen overflow-hidden bg-gray-50">
      {/* Sidebar */}
      <aside className="flex w-60 shrink-0 flex-col border-r border-gray-200 bg-white">
        <div className="flex h-14 items-center gap-2 border-b border-gray-200 px-4">
          <svg className="h-6 w-6 text-blue-600" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 3v11.25A2.25 2.25 0 006 16.5h2.25M3.75 3h-1.5m1.5 0h16.5m0 0h1.5m-1.5 0v11.25A2.25 2.25 0 0118 16.5h-2.25m-7.5 0h7.5m-7.5 0l-1 3m8.5-3 1 3" />
          </svg>
          <span className="text-sm font-bold text-gray-800">Chef Migration</span>
        </div>

        <nav className="flex-1 space-y-1 overflow-y-auto px-3 py-3">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              className={({ isActive }) =>
                `nav-link ${isActive ? "active" : ""}`
              }
            >
              <svg className="h-5 w-5 shrink-0" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" aria-hidden="true">
                <path strokeLinecap="round" strokeLinejoin="round" d={item.icon} />
              </svg>
              {item.label}
            </NavLink>
          ))}

          {/* Admin section — only visible to admin users */}
          {isAdmin && (
            <>
              <div className="my-2 border-t border-gray-200" />
              <div className="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wider text-gray-400">
                Admin
              </div>
              {adminNavItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    `nav-link ${isActive ? "active" : ""}`
                  }
                >
                  <svg className="h-5 w-5 shrink-0" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" aria-hidden="true">
                    <path strokeLinecap="round" strokeLinejoin="round" d={item.icon} />
                  </svg>
                  {item.label}
                </NavLink>
              ))}
            </>
          )}
        </nav>

        {/* Footer with health badge and user info */}
        <div className="border-t border-gray-200">
          {/* User info + logout */}
          {user && (
            <div className="flex items-center justify-between gap-2 border-b border-gray-200 px-4 py-2.5">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5">
                  {/* User avatar circle */}
                  <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-blue-100 text-xs font-semibold text-blue-700">
                    {(user.displayName || user.username).charAt(0).toUpperCase()}
                  </div>
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium text-gray-800">
                      {user.displayName || user.username}
                    </div>
                    <div className="flex items-center gap-1 text-[11px] text-gray-400">
                      <span className="truncate">{user.username}</span>
                      <span>·</span>
                      <span
                        className={`inline-flex shrink-0 items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium leading-none ${user.role === "admin"
                          ? "bg-purple-100 text-purple-700"
                          : "bg-gray-100 text-gray-600"
                          }`}
                      >
                        {user.role}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
              <button
                onClick={logout}
                title="Sign out"
                className="shrink-0 rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600"
              >
                <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" aria-hidden="true">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M15.75 9V5.25A2.25 2.25 0 0 0 13.5 3h-6a2.25 2.25 0 0 0-2.25 2.25v13.5A2.25 2.25 0 0 0 7.5 21h6a2.25 2.25 0 0 0 2.25-2.25V15m3 0 3-3m0 0-3-3m3 3H9"
                  />
                </svg>
              </button>
            </div>
          )}

          <div className="px-4 py-3">
            <HealthBadge />
          </div>
        </div>
      </aside>

      {/* Main content area */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex h-14 shrink-0 items-center justify-between border-b border-gray-200 bg-white px-6">
          <h1 className="text-lg font-semibold text-gray-800">Chef Migration Metrics</h1>
          <div className="w-64">
            <OrgSelector />
          </div>
        </header>

        {/* Page content (rendered by React Router) */}
        <main className="flex-1 overflow-y-auto p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
