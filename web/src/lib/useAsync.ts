import { useCallback, useEffect, useRef, useState } from "react";

export type AsyncState<T> = {
  data: T | undefined;
  error: Error | undefined;
  loading: boolean;
  reload: () => void;
};

// useAsync runs fn on mount and whenever a dep changes, tracking loading/error
// and giving back a manual reload().
export function useAsync<T>(fn: (signal: AbortSignal) => Promise<T>, deps: unknown[] = []): AsyncState<T> {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<Error>();
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  useEffect(() => {
    const ctrl = new AbortController();
    setLoading(true);
    fn(ctrl.signal)
      .then((d) => {
        if (mounted.current) {
          setData(d);
          setError(undefined);
        }
      })
      .catch((e) => {
        if (mounted.current && e.name !== "AbortError") setError(e as Error);
      })
      .finally(() => {
        if (mounted.current) setLoading(false);
      });
    return () => ctrl.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick, ...deps]);

  const reload = useCallback(() => setTick((t) => t + 1), []);
  return { data, error, loading, reload };
}

export type ActionState = {
  run: () => void;
  running: boolean;
  error: Error | undefined;
};

// useAction wraps a one-shot async action (button click) with running/error.
export function useAction(fn: () => Promise<unknown>, onDone?: () => void): ActionState {
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<Error>();

  const run = useCallback(() => {
    setRunning(true);
    setError(undefined);
    fn()
      .then(() => onDone?.())
      .catch((e) => setError(e as Error))
      .finally(() => setRunning(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fn]);

  return { run, running, error };
}
