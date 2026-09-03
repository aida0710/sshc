import { useId, useRef } from "react";
import { useTranslate } from "../i18n/context";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import type { TerminalPasteInspection } from "./pasteGuard";

export function TerminalPasteDialog({
  target,
  inspection,
  onCancel,
  onPaste,
  onPasteWithoutFinalLineBreak,
}: {
  target: string;
  inspection: TerminalPasteInspection;
  onCancel: () => void;
  onPaste: () => void;
  onPasteWithoutFinalLineBreak: () => void;
}) {
  const t = useTranslate();
  const cancel = useRef<HTMLButtonElement>(null);
  const headingID = useId();
  const descriptionID = useId();
  return (
    <ModalShell
      labelledBy={headingID}
      describedBy={descriptionID}
      onDismiss={onCancel}
      initialFocusRef={cancel}
      panelClassName="flex w-full max-w-2xl flex-col gap-3 rounded-lg p-4"
    >
      <h2 id={headingID} className="text-base font-semibold text-ink">
        {t("terminal.pasteHeading", { target })}
      </h2>
      <p id={descriptionID} className="text-sm text-ink-muted">
        {t("terminal.pasteDescription", { lines: String(inspection.lineCount) })}
      </p>
      <ul className="list-disc space-y-1 pl-5 text-sm text-notice-ink">
        {inspection.risks.includes("line-break") ? <li>{t("terminal.pasteRiskLineBreak")}</li> : null}
        {inspection.risks.includes("control-character") ? <li>{t("terminal.pasteRiskControl")}</li> : null}
      </ul>
      <pre
        aria-label={t("terminal.pastePreview")}
        className="max-h-64 overflow-auto whitespace-pre-wrap break-all rounded border border-control-line bg-control p-3 font-mono text-xs text-ink"
      >
        {inspection.preview}
      </pre>
      {inspection.previewTruncated ? (
        <p className="text-xs text-ink-muted">{t("terminal.pastePreviewTruncated")}</p>
      ) : null}
      <p className="text-xs font-medium text-notice-ink">{t("terminal.pasteNotSent")}</p>
      <div className="flex flex-wrap justify-end gap-2">
        <Button ref={cancel} onClick={onCancel}>{t("terminal.pasteCancel")}</Button>
        {inspection.endsWithLineBreak ? (
          <Button kind="primary" onClick={onPasteWithoutFinalLineBreak}>
            {t("terminal.pasteWithoutFinalLineBreak")}
          </Button>
        ) : null}
        <Button kind="danger" onClick={onPaste}>{t("terminal.pasteUnchanged")}</Button>
      </div>
    </ModalShell>
  );
}
