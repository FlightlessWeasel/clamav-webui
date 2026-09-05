import { useState } from "react";
import { setupAdmin } from "../api/client";
import { useAuth } from "../lib/AuthContext";
import { Alert, Button, Field } from "../components/ui";

export default function Setup() {
  const { refresh } = useAuth();
  const [pw, setPw] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (pw.length < 8) return setError("Password must be at least 8 characters.");
    if (pw !== confirm) return setError("Passwords do not match.");
    setBusy(true);
    setError(undefined);
    try {
      await setupAdmin(pw);
      await refresh();
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
    }
  };

  return (
    <div className="mx-auto mt-16 max-w-sm px-4">
      <h1 className="mb-1 text-xl font-semibold">Welcome to ClamAV WebUI</h1>
      <p className="mb-6 text-sm text-zinc-500">Set an admin password to finish setup.</p>
      <form onSubmit={submit} className="space-y-4">
        {error && <Alert>{error}</Alert>}
        <Field
          label="Admin password"
          type="password"
          autoComplete="new-password"
          value={pw}
          onChange={(e) => setPw(e.target.value)}
          autoFocus
        />
        <Field
          label="Confirm password"
          type="password"
          autoComplete="new-password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
        />
        <Button type="submit" disabled={busy} className="w-full">
          {busy ? "Saving…" : "Create admin password"}
        </Button>
      </form>
    </div>
  );
}
