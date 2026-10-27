import {
  useLayoutEffect,
  useRef,
  useState,
  type ClipboardEvent,
  type KeyboardEvent,
  type TextareaHTMLAttributes,
} from "react";

/** A textarea that grows with its text instead of scrolling. */
export function AutoTextarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  const ref = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const t = ref.current;
    if (!t) return;
    t.style.height = "auto";
    t.style.height = t.scrollHeight + "px";
  }, [props.value]);
  return <textarea ref={ref} rows={1} {...props} />;
}

interface InlineFieldProps {
  /** The saved value; shown whenever the field isn't being edited, so a refresh never overwrites typing. */
  value: string;
  /** Called on Enter or leaving the field when the text changed. */
  onSave: (value: string) => void;
  /** Esc reverts the edit; this runs instead when given, like closing a task just added. */
  onEscape?: () => boolean;
  /** Text that counts as unchanged, like the same tags in another order of spaces. */
  same?: (draft: string, saved: string) => boolean;
  multiline?: boolean;
  /** Enter (without Shift) saves, for one-line fields. */
  enterSaves?: boolean;
  onKeyDown?: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
  onPaste?: (e: ClipboardEvent<HTMLTextAreaElement>) => void;
  onBlurred?: () => void;
  className?: string;
  placeholder?: string;
  ariaLabel?: string;
  id?: string;
  autoFocus?: boolean;
}

/** A field edited in place: it saves when it loses focus, and Esc puts the saved text back. */
export function InlineField(p: InlineFieldProps) {
  const [draft, setDraft] = useState<string | null>(null);
  const cancelled = useRef(false);
  const same = p.same ?? ((a, b) => a.trim() === b.trim());

  const save = (text: string) => {
    if (!same(text, p.value)) p.onSave(text);
    setDraft(null);
  };

  return (
    <AutoTextarea
      id={p.id}
      className={p.className}
      placeholder={p.placeholder}
      aria-label={p.ariaLabel}
      autoFocus={p.autoFocus}
      spellCheck={p.multiline}
      value={draft ?? p.value}
      onFocus={() => setDraft(p.value)}
      onChange={(e) => setDraft(p.multiline ? e.target.value : e.target.value.replace(/\n/g, " "))}
      onPaste={p.onPaste}
      onBlur={() => {
        if (cancelled.current) cancelled.current = false;
        else if (draft !== null) save(draft);
        p.onBlurred?.();
      }}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.stopPropagation();
          if (p.onEscape?.()) return;
          cancelled.current = true;
          setDraft(null);
          e.currentTarget.blur();
          return;
        }
        if (p.enterSaves && e.key === "Enter" && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
          e.preventDefault();
          e.currentTarget.blur();
          return;
        }
        p.onKeyDown?.(e);
      }}
    />
  );
}

/** The image files in a paste or drop; text is left alone. */
export function imagesIn(data: DataTransfer | null): File[] {
  if (!data) return [];
  const fromItems = [...(data.items ?? [])].filter((i) => i.kind === "file").map((i) => i.getAsFile());
  const files = fromItems.length ? fromItems : [...(data.files ?? [])];
  return files.filter((f): f is File => !!f && f.type.startsWith("image/"));
}

export const hasFiles = (data: DataTransfer | null) => !!data && [...data.types].includes("Files");
