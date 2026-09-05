import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { getStatus, type Status } from "../api/client";

type AuthValue = {
  status: Status | undefined;
  loading: boolean;
  refresh: () => Promise<void>;
};

const AuthCtx = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<Status>();
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      setStatus(await getStatus());
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return <AuthCtx.Provider value={{ status, loading, refresh }}>{children}</AuthCtx.Provider>;
}

export function useAuth(): AuthValue {
  const v = useContext(AuthCtx);
  if (!v) throw new Error("useAuth outside AuthProvider");
  return v;
}
