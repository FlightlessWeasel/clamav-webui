import { useState } from "react";
import { changePassword } from "../api/client";
import { useAuth } from "../lib/AuthContext";
import { Alert, Button, Card, Field } from "./ui";

export default function PasswordSettings() {
  const { refresh } = useAuth();
  const [cur, setCur] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [err, setErr] = useState<string>();
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (next.length < 8) return setErr("New password must be at least 8 characters.");
    if (next !== confirm) return setErr("New passwords do not match.");
    setBusy(true);
    setErr(undefined);
    try {
      await changePassword(cur, next);
      // The session is now invalid server-side; refresh drops us to the login screen.
      await refresh();
    } catch (e2) {
      setErr((e2 as Error).message);
      setBusy(false);
    }
  };

  return (
    <Card title="Admin password">
      <form onSubmit={submit} className="max-w-sm space-y-3">
        {err && <Alert>{err}</Alert>}
        <Field label="Current password" type="password" value={cur} onChange={(e) => setCur(e.target.value)} />
        <Field label="New password" type="password" value={next} onChange={(e) => setNext(e.target.value)} />
        <Field
          label="Confirm new password"
          type="password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
        />
        <Button type="submit" disabled={busy}>
          {busy ? "Changing…" : "Change password"}
        </Button>
        <p className="text-xs text-zinc-500">Changing the password signs out every session.</p>
      </form>
    </Card>
  );
}
