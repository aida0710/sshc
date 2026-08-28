import { useEffect, useState } from "react";
import { CopyButton } from "../ui/CopyButton";
import { useTranslate } from "../i18n/context";
import type { KeysApi } from "./api";

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

  function close() {
    setMaterial("");
    setState("confirm");
    onClose();
  }

  useEffect(() => {
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") close();
    }
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  });

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
    <div
      role="dialog"
      aria-labelledby="reveal-heading"
      className="rounded-md border border-line bg-surface-subtle p-4 sm:p-5"
    >
      <h3 id="reveal-heading" className="font-medium">
        {t("reveal.heading", { path: relativePath })}
      </h3>
      {state === "confirm" && (
        <>
          <p className="mt-3 rounded-md bg-notice px-3 py-2 text-sm text-notice-ink">{t("reveal.warning")}</p>
          <button
            type="button"
            className="mt-4 rounded-md border border-control-line bg-card px-3 py-2 text-sm font-medium hover:bg-select-fill"
            onClick={() => void confirm()}
          >
            {t("reveal.show")}
          </button>
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
      <button type="button" className="mt-4 rounded-md border border-control-line px-3 py-2" onClick={close}>
        {t("reveal.close")}
      </button>
    </div>
  );
}
