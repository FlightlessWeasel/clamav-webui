import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, useAuth } from "./lib/AuthContext";
import { Spinner } from "./components/ui";
import Layout from "./components/Layout";
import Setup from "./pages/Setup";
import Login from "./pages/Login";
import Dashboard from "./pages/Dashboard";
import Install from "./pages/Install";
import Signatures from "./pages/Signatures";
import Scan from "./pages/Scan";
import Scans from "./pages/Scans";
import ScanDetail from "./pages/ScanDetail";
import Stub from "./pages/Stub";

function Gate() {
  const { status, loading } = useAuth();

  if (loading || !status) {
    return (
      <div className="p-8">
        <Spinner />
      </div>
    );
  }
  if (!status.setup_complete) return <Setup />;
  if (!status.authenticated) return <Login />;

  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Dashboard />} />
        <Route path="install" element={<Install />} />
        <Route path="scan" element={<Scan />} />
        <Route path="scans" element={<Scans />} />
        <Route path="scans/:id" element={<ScanDetail />} />
        <Route path="schedules" element={<Stub title="Schedules" />} />
        <Route path="signatures" element={<Signatures />} />
        <Route path="quarantine" element={<Stub title="Quarantine" />} />
        <Route path="protection" element={<Stub title="Protection" />} />
        <Route path="activity" element={<Stub title="Activity" />} />
        <Route path="settings" element={<Stub title="Settings" />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Gate />
      </AuthProvider>
    </BrowserRouter>
  );
}
