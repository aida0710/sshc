import { Spinner } from "../ui/Spinner";

// SFTPWaitingOverlay covers the rows while the pane waits for something it
// cannot show in them yet: a directory being read, or a file being opened in
// the editor. `detail` names which one, so a slow host is not mistaken for a
// press that was not taken.
export function SFTPWaitingOverlay({ label, detail }: { label: string; detail: string }) {
  return (
    <div role="status" className="absolute inset-0 z-10 flex items-center justify-center rounded-md bg-card/80 p-4 backdrop-blur-[1px]">
      <span className="flex min-w-0 items-center gap-3 rounded-lg bg-toolbar px-4 py-3 text-sm">
        <Spinner className="size-4 text-accent" />
        <span className="min-w-0">
          <span className="block">{label}</span>
          <span className="block truncate font-mono text-xs text-ink-muted">{detail}</span>
        </span>
      </span>
    </div>
  );
}
