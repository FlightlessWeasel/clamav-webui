import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { logout } from "../api/client";
import { useAuth } from "../lib/AuthContext";
import { Button } from "./ui";

const NAV = [
  { to: "/", label: "Dashboard", end: true },
  { to: "/scan", label: "Scan" },
  { to: "/scans", label: "History" },
  { to: "/schedules", label: "Schedules" },
  { to: "/signatures", label: "Signatures" },
  { to: "/quarantine", label: "Quarantine" },
  { to: "/protection", label: "Protection" },
  { to: "/activity", label: "Activity" },
  { to: "/settings", label: "Settings" },
];

export default function Layout() {
  const { status, refresh } = useAuth();
  const navigate = useNavigate();

  const signOut = async () => {
    try {
      await logout();
    } finally {
      await refresh();
      navigate("/");
    }
  };

  return (
    <div className="min-h-full bg-zinc-50 text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100">
      <header className="border-b border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
        <div className="mx-auto flex max-w-6xl items-center gap-4 px-4 py-3">
          <span className="font-semibold">ClamAV WebUI</span>
          <nav className="flex flex-wrap gap-1 text-sm">
            {NAV.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                end={n.end}
                className={({ isActive }) =>
                  `rounded px-2 py-1 ${
                    isActive
                      ? "bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-200"
                      : "hover:bg-zinc-100 dark:hover:bg-zinc-800"
                  }`
                }
              >
                {n.label}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-3 text-xs text-zinc-500">
            <span>v{status?.version}</span>
            <Button variant="secondary" onClick={signOut}>
              Sign out
            </Button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-6xl space-y-4 px-4 py-6">
        <Outlet />
      </main>
    </div>
  );
}
