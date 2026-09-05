import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, useAuth } from "./lib/AuthContext";
import { Spinner } from "./components/ui";
import Layout from "./components/Layout";
import Setup from "./pages/Setup";
import Login from "./pages/Login";
import Dashboard from "./pages/Dashboard";

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
