import { useEffect, useRef } from "react";

export type ServerEvent = { type: string; data: unknown };

// useEvents opens an EventSource to /api/events and calls onEvent for every
// named event. The handler is kept in a ref so the stream is not torn down when
// the caller passes a fresh closure each render.
export function useEvents(onEvent: (ev: ServerEvent) => void, types: string[] = []) {
  const handler = useRef(onEvent);
  handler.current = onEvent;
  const key = types.join(",");

  useEffect(() => {
    const es = new EventSource("/api/events", { withCredentials: true });
    const names = key ? key.split(",") : ["job", "job-log", "scan", "scan-finding", "service"];

    const listener = (name: string) => (e: MessageEvent) => {
      let data: unknown = e.data;
      try {
        data = JSON.parse(e.data);
      } catch {
        /* keep raw */
      }
      handler.current({ type: name, data });
    };

    const bound = names.map((n) => {
      const fn = listener(n);
      es.addEventListener(n, fn as EventListener);
      return [n, fn] as const;
    });

    return () => {
      bound.forEach(([n, fn]) => es.removeEventListener(n, fn as EventListener));
      es.close();
    };
  }, [key]);
}
