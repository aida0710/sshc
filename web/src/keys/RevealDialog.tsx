import { useRef, useState } from "react";
import { CopyButton } from "../ui/CopyButton";
import { useTranslate } from "../i18n/context";
import type { KeysApi } from "./api";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";

type RevealDialogProps = {
  keyId: string;
  relativePath: string;
  api: Pick<KeysApi, "reveal">;
  onClose: () => void;
};

type DialogState = "confirm" | "loading" | "shown" | "error";

export function RevealDialog({ keyId, relativePath, api, onClose }: RevealDialogProps) {
  const t = useTranslate();
  const [state, setState] = useState<DialogState>("confirm");
  const [material, setMaterial] = useState("");
  const showButton = useRef<HTMLButtonElement>(null);

  function close() {
    setMaterial("");
    setState("confirm");
    onClose();
  }

  async function confirm() {
    setState("loading");
    try {
      const response = await api.reveal(keyId);
      setMaterial(response.privateKey);
      setState("shown");
    } catch {
      setMaterial("");
      setState("error");
    }
  }

  return (
    <ModalShell
      labelledBy="reveal-heading"
      onDismiss={close}
      initialFocusRef={showButton}
      panelClassName="w-full max-w-2xl rounded-lg p-4 sm:p-5"
    >
      <h3 id="reveal-heading" className="font-medium">
        {t("reveal.heading", { path: relativePath })}
      </h3>
      {state === "confirm" && (
        <>
          <p className="mt-3 rounded-md bg-notice px-3 py-2 text-sm text-notice-ink">{t("reveal.warning")}</p>
          <Button
            ref={showButton}
            className="mt-4"
            onClick={() => void confirm()}
          >
            {t("reveal.show")}
          </Button>
        </>
      )}
      {state === "loading" && (
        <p aria-live="polite" className="mt-2 text-sm text-ink-muted">
          {t("reveal.requesting")}
        </p>
      )}
      {state === "shown" && (
        <>
          <pre aria-label={t("reveal.privateKeyLabel")} className="mt-4 overflow-x-auto rounded-md bg-canvas p-4 text-xs">
            {material}
          </pre>

          <div className="mt-2">
            <CopyButton value={material} label="copy.privateKey" />
          </div>
        </>
      )}
      {state === "error" && (
        <p role="alert" className="mt-2 text-sm text-danger">
          {t("reveal.failed")}
        </p>
      )}
      <Button className="mt-4" onClick={close}>
        {t("reveal.close")}
      </Button>
    </ModalShell>
  );
}
