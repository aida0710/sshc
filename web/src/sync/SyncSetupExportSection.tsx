import { useEffect, useRef, useState } from "react";
import { AndroidSaveCancelled } from "../android/native";
import type { SyncSetupExport } from "../api/sync";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { PageSection } from "../ui/page";
import { QRCode } from "../ui/QRCode";
import { saveFile, type BrowserSave } from "../ui/saveFile";
import { Button, Notice } from "../ui/surface";
import { syncSetupExportFileName } from "./syncSetupExportDocument";

// Handing this machine's sync settings to another machine. The engine seals them with the encryption key it
// already holds, so nothing is typed here; the other machine types the key to open them. exported is the
// sealed document once the engine has made one, and stays on the screen until the person closes it.
export function SyncSetupExportSection({ busy, exported, onExport, onClose }: {
  busy: boolean;
  exported: SyncSetupExport | null;
  onExport: () => void;
  onClose: () => void;
}) {
  const t = useTranslate();
  const [saveFailed, setSaveFailed] = useState(false);
  // The browser may read the saved file after the click; it is let go when the next save replaces it or the
  // section goes away.
  const lastSave = useRef<BrowserSave | null>(null);
  useEffect(() => () => lastSave.current?.release(), []);

  function save(document: string) {
    setSaveFailed(false);
    // The Android app takes a bare media type for its save dialog (NativeBridge.safeMimeType), so no charset.
    void saveFile(new Blob([document], { type: "application/json" }), syncSetupExportFileName)
      .then((browserSave) => {
        lastSave.current?.release();
        lastSave.current = browserSave;
      })
      .catch((error: unknown) => {
        if (!(error instanceof AndroidSaveCancelled)) setSaveFailed(true);
      });
  }

  return (
    <PageSection
      heading={t("sync.export.heading")}
      hint={t("sync.export.hint")}
      actions={exported === null ? <Button busy={busy} onClick={onExport}>{t("sync.export.start")}</Button> : undefined}
    >
      {exported === null ? null : (
        <div className="flex flex-col items-start gap-3">
          {exported.keyGenerated ? null : <Notice tone="notice">{t("sync.export.chosenKeyWarning")}</Notice>}
          {exported.qrRows === undefined ? (
            <p className="text-sm text-ink-muted">{t("sync.export.tooLongForQR")}</p>
          ) : (
            <>
              <QRCode rows={exported.qrRows} label={t("sync.export.qrLabel")} className="w-full max-w-80 rounded-inset" />
              <p className="text-xs leading-[18px] text-ink-faint">{t("sync.export.qrHint")}</p>
            </>
          )}
          {saveFailed ? <Notice tone="danger">{t("sync.export.saveFailed")}</Notice> : null}
          <div className="flex flex-wrap gap-2">
            <Button className="inline-flex items-center gap-1.5" onClick={() => save(exported.document)}>
              <Icon name="download" />
              {t("sync.export.saveFile")}
            </Button>
            <Button onClick={onClose}>{t("sync.export.close")}</Button>
          </div>
        </div>
      )}
    </PageSection>
  );
}
