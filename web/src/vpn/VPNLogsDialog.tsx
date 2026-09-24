import { useEffect, useRef, useState } from "react";
import type { VPNApi } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";

// engine が経路を用意した記録と、コンテナの直近の出力を見せる。繋がらないときに
// 最初に見る場所で、利用者に docker を直接叩かせないためにある。秘密は engine の
// 側で伏せてある。
export function VPNLogsDialog({
  name,
  api,
  describe,
  onClose,
}: {
  name: string;
  api: VPNApi;
  // describe は、engine の断り方をこの画面の言い方へ直す。
  describe: (error: unknown) => string;
  onClose: () => void;
}) {
  const t = useTranslate();
  const [lines, setLines] = useState<string | null>(null);
  const [failure, setFailure] = useState("");
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    let current = true;
    void api
      .vpnLogs(name)
      .then((logs) => {
        if (current) setLines(logs.lines);
      })
      .catch((error: unknown) => {
        if (current) setFailure(describe(error));
      });
    return () => {
      current = false;
    };
  }, [api, describe, name]);

  return (
    <ModalShell
      labelledBy="vpn-logs"
      onDismiss={onClose}
      initialFocusRef={closeRef}
      panelClassName="flex w-full max-w-3xl flex-col gap-3 rounded-lg p-4"
    >
      <div>
        <h2 id="vpn-logs" className="text-base font-semibold text-ink">
          {t("vpn.logsTitle", { name })}
        </h2>
        <p className={`mt-1 ${hintText}`}>{t("vpn.logsHint")}</p>
      </div>
      {failure !== "" ? (
        <p role="alert" className="text-sm text-danger">
          {failure}
        </p>
      ) : lines === null ? (
        <p className={hintText}>{t("vpn.logsLoading")}</p>
      ) : lines.trim() === "" ? (
        <p className={hintText}>{t("vpn.logsEmpty")}</p>
      ) : (
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded border border-line bg-surface-subtle p-3 text-xs text-ink">
          {lines}
        </pre>
      )}
      <div className="flex justify-end">
        <Button ref={closeRef} onClick={onClose}>
          {t("vpn.close")}
        </Button>
      </div>
    </ModalShell>
  );
}
