import { Notice } from "../ui/surface";

export function SyncErrorNotice({ message, code }: { message: string; code: string }) {
  if (message === "") return null;
  return (
    <Notice tone="danger">
      <span className="flex min-w-0 flex-col gap-1">
        <span>{message}</span>
        {code === "" ? null : (
          <code className="text-xs text-ink-muted">Code: {code}</code>
        )}
      </span>
    </Notice>
  );
}
