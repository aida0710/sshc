import { useRef } from "react";
import { useTranslate } from "../i18n/context";
import { FieldRow } from "../ui/FieldRow";
import { Icon } from "../ui/icons";
import { PageSection } from "../ui/page";
import { PasswordInput } from "../ui/PasswordField";
import { QRCodeScanner } from "../ui/QRCodeScanner";
import { Button, Notice } from "../ui/surface";
import { cameraAvailable } from "../ui/useQRCodeCamera";
import { SyncDirectionField } from "./SyncDirectionField";
import type { SyncSetupImport } from "./useSyncSetupImport";

// Setting this machine up from the settings another machine exported: a file or a QR code, then the
// encryption key. The engine opens the document with the key, checks the bucket, and saves both.
export function SyncSetupImportSection({ busy, form, onImport }: {
  busy: boolean;
  form: SyncSetupImport;
  onImport: () => void;
}) {
  const t = useTranslate();
  const chooser = useRef<HTMLInputElement>(null);
  return (
    <PageSection heading={t("sync.import.heading")} hint={t("sync.import.hint")}>
      {form.document === "" ? (
        <div className="flex flex-col gap-3">
          {form.scanning ? (
            <QRCodeScanner label={t("sync.import.scanLabel")} accept={form.acceptScannedText} onCancel={form.stopScanning} />
          ) : (
            <div className="flex flex-wrap gap-2">
              {/* 拡張子を含む accept にする。Android は .json の MIME type が当てにならず、MIME type だけで
                  絞ると保存したファイルを選べないことがある（FileChooser.mimeTypes）。 */}
              <input
                ref={chooser}
                type="file"
                className="hidden"
                accept=".json,application/json"
                onChange={(event) => {
                  const file = event.target.files?.[0];
                  event.target.value = "";
                  if (file !== undefined) void form.readFile(file);
                }}
              />
              <Button className="inline-flex items-center gap-1.5" onClick={() => chooser.current?.click()}>
                <Icon name="openFile" />
                {t("sync.import.chooseFile")}
              </Button>
              {cameraAvailable() ? (
                <Button className="inline-flex items-center gap-1.5" onClick={form.startScanning}>
                  <Icon name="qrCode" />
                  {t("sync.import.scan")}
                </Button>
              ) : null}
            </div>
          )}
          {form.fileProblem === null ? null : <Notice tone="danger">{t(form.fileProblem)}</Notice>}
        </div>
      ) : (
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            onImport();
          }}
        >
          <Notice tone="success">{t("sync.import.loaded")}</Notice>
          <div>
            <FieldRow label={t("sync.key")} interactiveChildren>
              <PasswordInput label={t("sync.key")} value={form.key} onChange={form.setKey} />
            </FieldRow>
            <SyncDirectionField direction={form.direction} onChange={form.setDirection} />
          </div>
          <div className="flex flex-wrap justify-end gap-2">
            <Button disabled={busy} onClick={form.reset}>
              {t("sync.import.chooseAnother")}
            </Button>
            <Button kind="primary" type="submit" busy={busy} disabled={form.key.trim() === ""}>
              {t("sync.import.submit")}
            </Button>
          </div>
        </form>
      )}
    </PageSection>
  );
}
