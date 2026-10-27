import { useSyncExternalStore } from "react";

// One shared minute clock, so relative times like "5m ago" stay current.
let now = Date.now();
const listeners = new Set<() => void>();
let timer: ReturnType<typeof setInterval> | undefined;

function subscribe(listener: () => void) {
  listeners.add(listener);
  if (!timer) now = Date.now();
  timer ??= setInterval(() => {
    now = Date.now();
    listeners.forEach((l) => l());
  }, 60_000);
  return () => {
    listeners.delete(listener);
    if (!listeners.size) {
      clearInterval(timer);
      timer = undefined;
    }
  };
}

export const useNow = () => useSyncExternalStore(subscribe, () => now);
