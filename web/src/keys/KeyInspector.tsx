import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { noteLabels } from "./labels";
import { certificateLines } from "./KeyTable";
import type { KeyItem } from "./api";

// 表は探すためのもの、ここは見るためのもの。
// 指紋も権限も、一覧を走査するときには読まない。

export function KeyInspector({ item, now }: { item: KeyItem; now: number }) {
  const t = useTranslate();
  const references = item.references.map((reference) => reference.hostPatterns.join(" "));

  return (
    <div className="flex flex-col gap-4">
      <dl className="flex flex-col gap-2 text-sm">
        <Fact
          label={t("keys.colFile")}
          value={<span className="font-mono text-xs break-all">{item.relativePath}</span>}
        />
        <Fact
          label={t("keys.colAlgorithm")}
          value={item.bits > 0 ? `${item.algorithm} · ${item.bits}` : item.algorithm}
        />
        {item.fingerprint === "" ? null : (
          <Fact
            label={t("keys.colFingerprint")}
            value={<span className="font-mono text-xs break-all">{item.fingerprint}</span>}
          />
        )}
        <Fact
          label={t("keys.colPermissions")}
          value={
            <>
              {item.permission}
              {item.permissionRisk && (
                <span className="ml-2 text-notice-ink">{t("keys.permissionRisk")}</span>
              )}
            </>
          }
        />
        {/* 消してよいかを決める人が要るのは、数ではなく名前である。 */}
        <Fact
          label={t("keys.colUsedBy")}
          value={references.length === 0 ? t("keys.usedByNothing") : references.join(", ")}
        />
      </dl>

      {item.notes.length === 0 ? null : (
        <ul className="flex flex-col gap-1 text-xs text-notice-ink">
          {item.notes.map((note) => (
            <li key={note}>{note in noteLabels ? t(noteLabels[note]!) : note}</li>
          ))}
        </ul>
      )}

      {item.certificate === undefined ? null : (
        <ul className="flex flex-col gap-1 text-xs text-ink-muted">
          {certificateLines(item.certificate, now, t).map((line) => (
            <li key={line.text} className={line.expired ? "text-danger" : undefined}>
              {line.text}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function Fact({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-xs text-ink-muted">{label}</dt>
      <dd className="text-ink">{value}</dd>
    </div>
  );
}
