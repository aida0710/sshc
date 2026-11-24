import { useTranslate } from "../i18n/context";
import { Notice } from "../ui/surface";

export function SyncErrorNotice({ message, code }: { message: string; code: string }) {
  const t = useTranslate();
  if (message === "") return null;
  // The code is for a bug report or a search, not for reading first.
  const details = code === "" ? undefined : { label: t("ui.moreDetails"), content: <code className="text-xs text-ink-muted">{code}</code> };
  return <Notice tone="danger" details={details}>{message}</Notice>;
}
