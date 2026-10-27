import { createContext, use, useCallback, useRef, useState, type ReactNode } from "react";
import { AlertDialog } from "radix-ui";

export interface ConfirmOptions {
  title: string;
  body?: ReactNode;
  /** The label of the button that goes ahead. */
  action: string;
  danger?: boolean;
  /** A second way to go ahead, like "Overwrite the remote" next to "Use the remote". */
  alternative?: string;
}

/** "action", "alternative", or null for cancel. */
export type Choice = "action" | "alternative" | null;

const Ctx = createContext<(o: ConfirmOptions) => Promise<Choice>>(() => Promise.resolve(null));

/** Asks the person to confirm, in place of window.confirm. */
export const useConfirm = () => use(Ctx);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [options, setOptions] = useState<ConfirmOptions | null>(null);
  const resolve = useRef<(c: Choice) => void>(() => {});

  const ask = useCallback(
    (o: ConfirmOptions) =>
      new Promise<Choice>((r) => {
        resolve.current = r;
        setOptions(o);
      }),
    [],
  );
  const answer = (c: Choice) => {
    resolve.current(c);
    setOptions(null);
  };

  return (
    <Ctx value={ask}>
      {children}
      <AlertDialog.Root open={!!options} onOpenChange={(open) => !open && answer(null)}>
        <AlertDialog.Portal>
          <AlertDialog.Overlay className="fixed inset-0 z-50 bg-black/40" />
          <AlertDialog.Content className="fixed top-[20vh] left-1/2 z-50 flex w-[min(460px,calc(100vw-32px))] -translate-x-1/2 flex-col gap-3 rounded-2xl border border-line bg-menu p-5 shadow-pop">
            <AlertDialog.Title className="font-display text-lg font-semibold">{options?.title}</AlertDialog.Title>
            {options?.body && (
              <AlertDialog.Description className="whitespace-pre-line text-muted">
                {options.body}
              </AlertDialog.Description>
            )}
            <div className="mt-2 flex flex-wrap justify-end gap-2">
              <AlertDialog.Cancel className="btn">Cancel</AlertDialog.Cancel>
              {options?.alternative && (
                <button className="btn" onClick={() => answer("alternative")}>
                  {options.alternative}
                </button>
              )}
              <AlertDialog.Action
                className={options?.danger ? "btn btn-danger" : "btn btn-primary"}
                onClick={() => answer("action")}
              >
                {options?.action}
              </AlertDialog.Action>
            </div>
          </AlertDialog.Content>
        </AlertDialog.Portal>
      </AlertDialog.Root>
    </Ctx>
  );
}
