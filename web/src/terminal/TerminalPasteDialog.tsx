import { useId, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { inspectTerminalPaste, removeFinalTerminalLineBreak } from "./pasteGuard";

export function TerminalPasteDialog({
  target,
  text,
  onCancel,
  onPaste,
}: {
  target: string;
  text: string;
  onCancel: () => void;
  onPaste: (text: string) => void;
}) {
  const t = useTranslate();
  const [draft, setDraft] = useState(text);
  const inspection = inspectTerminalPaste(draft);
  const cancel = useRef<HTMLButtonElement>(null);
  const headingID = useId();
  const descriptionID = useId();
  return (
    <ModalShell
      labelledBy={headingID}
      describedBy={descriptionID}
      onDismiss={onCancel}
      initialFocusRef={cancel}
      panelClassName="flex max-h-[90dvh] overflow-y-auto w-full max-w-2xl flex-col gap-3 rounded-lg p-4"
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
      <label className="flex flex-col gap-1 text-sm text-ink">
        {t("terminal.pasteEdit")}
        <textarea
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          rows={6}
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          className="w-full resize-y rounded border border-control-line bg-control p-3 font-mono text-xs focus:border-accent focus:outline-none"
        />
      </label>
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
          <Button kind="primary" onClick={() => onPaste(removeFinalTerminalLineBreak(draft))}>
            {t("terminal.pasteWithoutFinalLineBreak")}
          </Button>
        ) : null}
        <Button kind="danger" onClick={() => onPaste(draft)}>{t("terminal.pasteSend")}</Button>
      </div>
    </ModalShell>
  );
}
