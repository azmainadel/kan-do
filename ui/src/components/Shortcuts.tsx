import { Dialog } from "radix-ui";

const KEYS: [string[], string][] = [
  [["/", "⌘K"], "Search every project"],
  [["n"], "New task"],
  [["f"], "Filter the cards"],
  [["←", "↑", "↓", "→"], "Move between cards (or h, k, j, l)"],
  [["Enter"], "Open the focused card"],
  [["Shift", "← →"], "Move the card to the next column"],
  [["Shift", "↑ ↓"], "Move the card up or down"],
  [["c"], "Copy the card's key, like api#12"],
  [["Esc"], "Close the task, or leave the board's focus"],
  [["⌘↵"], "Save and close, or send a comment"],
  [["?"], "These shortcuts"],
];

export function Shortcuts({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/45" />
        <Dialog.Content
          aria-describedby={undefined}
          className="fixed top-[12vh] left-1/2 z-50 w-[min(440px,calc(100vw-32px))] -translate-x-1/2 rounded-[18px] border border-line bg-menu p-5 shadow-pop focus-visible:outline-none"
        >
          <Dialog.Title className="mb-3 font-display text-lg font-semibold">Keyboard shortcuts</Dialog.Title>
          <dl className="grid grid-cols-[auto_1fr] items-center gap-x-5 gap-y-2">
            {KEYS.map(([keys, what]) => (
              <div key={what} className="contents">
                <dt className="flex justify-end gap-1">
                  {keys.map((k) => (
                    <kbd
                      key={k}
                      className="min-w-6 rounded-md border border-line bg-field px-1.5 py-px text-center font-mono text-xs"
                    >
                      {k}
                    </kbd>
                  ))}
                </dt>
                <dd className="text-muted">{what}</dd>
              </div>
            ))}
          </dl>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
