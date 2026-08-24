import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { noteLabels } from "./labels";
import { certificateLines } from "./KeyTable";
import type { KeyItem } from "./api";

export function KeyInspector({ item, now }: { item: KeyItem; now: number }) {
  const t = useTranslate();
  const references = item.references.map((reference) => reference.hostPatterns.join(" "));

  return (
    <div className="flex flex-col gap-4">
      <header className="rounded-lg bg-surface-subtle p-3">
        <p className="break-all font-mono text-sm font-semibold text-ink">{item.relativePath}</p>
        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-ink-muted">
          <span className="rounded-md bg-surface px-2 py-1">{item.kind}</span>
          <span className="font-mono">
            {item.bits > 0 ? `${item.algorithm} · ${item.bits}` : item.algorithm}
          </span>
        </div>
      </header>
      <dl className="grid gap-3 text-sm">
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
    <div className="grid gap-0.5 border-b border-hairline pb-3 last:border-b-0 last:pb-0">
      <dt className="text-[11px] font-medium uppercase tracking-wide text-ink-muted">{label}</dt>
      <dd className="text-ink">{value}</dd>
    </div>
  );
}
