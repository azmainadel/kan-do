import { useState } from "react";
import { Dialog } from "radix-ui";
import { api } from "../api";

interface Props {
  open: boolean;
  remote: string | null;
  onOpenChange: (open: boolean) => void;
  /** The repo was saved; sync with it now. */
  onSaved: (remote: string) => void;
}

/** Sync asks for a repo the first time; after that, the repo in the sidebar changes it. */
export function SyncDialog({ open, remote, onOpenChange, onSaved }: Props) {
  const [url, setUrl] = useState(remote ?? "");
  const [error, setError] = useState("");
  const [checking, setChecking] = useState(false);

  const save = async () => {
    setChecking(true);
    setError("");
    try {
      const { remote } = await api.setSyncRemote(url);
      onOpenChange(false);
      onSaved(remote);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
    setChecking(false);
  };

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(o) => {
        if (o) {
          setUrl(remote ?? "");
          setError("");
        }
        onOpenChange(o);
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/45" />
        <Dialog.Content className="fixed top-[12vh] left-1/2 z-50 w-[min(520px,calc(100vw-32px))] -translate-x-1/2 rounded-[18px] border border-line bg-menu p-[22px] pb-[18px] shadow-pop">
          <form
            className="flex flex-col gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              save();
            }}
          >
            <Dialog.Title className="font-display text-lg font-semibold">
              {remote ? "Sync with another repo" : "Sync your board"}
            </Dialog.Title>
            <Dialog.Description className="text-muted">
              Keep this board the same on all your machines through a private git repo you own. Use the same repo on
              each machine.
            </Dialog.Description>
            <input
              className="field"
              placeholder="https://github.com/you/kan-do-data"
              aria-label="Repository URL"
              autoComplete="off"
              spellCheck={false}
              autoFocus
              onFocus={(e) => e.target.select()}
              value={url}
              onChange={(e) => setUrl(e.target.value)}
            />
            {error && <p className="text-[13px] whitespace-pre-line text-danger">{error}</p>}
            <div className="mt-1 flex flex-wrap items-center gap-2">
              {!remote && (
                <a
                  className="mr-auto text-[13px] text-muted underline-offset-2 hover:text-text hover:underline"
                  href="https://github.com/new?name=kan-do-data&visibility=private"
                  target="_blank"
                  rel="noopener"
                >
                  Create a private repo on GitHub
                </a>
              )}
              <Dialog.Close className={`btn ${remote ? "ml-auto" : ""}`} type="button">
                Cancel
              </Dialog.Close>
              <button className="btn btn-primary" type="submit" disabled={checking}>
                {checking ? "Checking…" : "Save and sync"}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
