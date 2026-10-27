import { useCallback, useState } from "react";

/** localStorage that never throws: private windows and blocked storage just forget. */
export const storage = {
  get(key: string): string | null {
    try {
      return localStorage.getItem(key);
    } catch {
      return null;
    }
  },
  set(key: string, value: string) {
    try {
      localStorage.setItem(key, value);
    } catch {
      // Remembering is a convenience; the board works without it.
    }
  },
};

/** A boolean this browser remembers. */
export function useStoredFlag(key: string, fallback: boolean): [boolean, (v: boolean) => void] {
  const [value, setValue] = useState(() => {
    const stored = storage.get(key);
    return stored === null ? fallback : stored === "1";
  });
  const set = useCallback(
    (v: boolean) => {
      storage.set(key, v ? "1" : "0");
      setValue(v);
    },
    [key],
  );
  return [value, set];
}
