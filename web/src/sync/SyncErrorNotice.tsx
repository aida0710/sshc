import { MoreDetails } from "../ui/MoreDetails";
import { Notice } from "../ui/surface";

export function SyncErrorNotice({ message, code }: { message: string; code: string }) {
  if (message === "") return null;
  return (
    <Notice tone="danger">
      <div className="flex min-w-0 flex-col gap-1">
        <span>{message}</span>
        {/* The code is for a bug report or a search, not for reading first. */}
        {code === "" ? null : <MoreDetails><code>{code}</code></MoreDetails>}
      </div>
    </Notice>
  );
}
