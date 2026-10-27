import { useEffect, useRef, useState } from "react";

/**
 * Listens to the board's change stream and calls onChange with each new token. Returns
 * whether the stream is connected; EventSource reconnects by itself, and the first event
 * after a reconnect carries the current token, so nothing is missed while offline.
 */
export function useEvents(onChange: (token: string) => void): boolean {
  const [online, setOnline] = useState(true);
  const latest = useRef(onChange);
  useEffect(() => {
    latest.current = onChange;
  });

  useEffect(() => {
    const es = new EventSource("/api/events");
    es.addEventListener("change", (e) => {
      setOnline(true);
      latest.current((e as MessageEvent<string>).data);
    });
    es.onopen = () => setOnline(true);
    es.onerror = () => setOnline(false);
    return () => es.close();
  }, []);

  return online;
}
