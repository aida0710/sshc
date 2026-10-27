import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { describeAlgorithm, describeKeyKind, noteLabels } from "./labels";
import { certificateLines } from "./KeyTable";
import type { KeyItem } from "./api";

// KeyInspector is the details pane of a key chosen in the table: what the row has no room for, with the
// public key and certificate files that go with a private key (related).
export function KeyInspector({ item, related, now }: { item: KeyItem; related: KeyItem[]; now: number }) {
  const t = useTranslate();
  const references = item.references.map((reference) => reference.hostPatterns.join(" "));

  return (
    <div className="flex flex-col gap-4">
      <header className="rounded-lg bg-surface-subtle p-3">
        <p className="break-all font-mono text-sm font-semibold text-ink">{item.relativePath}</p>
        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-ink-muted">
          <span className="rounded-md bg-surface px-2 py-1">{describeKeyKind(item.kind, t)}</span>
          <span className="font-mono">{describeAlgorithm(item.algorithm, item.bits)}</span>
        </div>
      </header>
      <dl className="grid gap-3 text-sm">
        {item.fingerprint === "" ? null : (
          <Fact
            label={t("keys.colFingerprint")}
            value={<span className="font-mono text-xs break-all">{item.fingerprint}</span>}
          />
        )}
        {item.comment === "" ? null : <Fact label={t("keys.colComment")} value={item.comment} />}
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
        {related.length === 0 ? null : (
          <Fact
            label={t("keys.relatedFiles")}
            value={
              <ul className="flex flex-col gap-2">
                {related.map((file) => (
                  <li key={file.id}>
                    <span className="block break-all font-mono text-xs">{file.relativePath}</span>
                    {file.certificate === undefined ? null : (
                      <ul className="mt-0.5 break-all text-xs text-ink-muted">
                        {certificateLines(file.certificate, now, t).map((line) => (
                          <li key={line.text} className={line.expired ? "text-danger" : undefined}>{line.text}</li>
                        ))}
                      </ul>
                    )}
                  </li>
                ))}
              </ul>
            }
          />
        )}
      </dl>

      {item.notes.length === 0 ? null : (
        <ul className="flex flex-col gap-1 text-xs text-notice-ink">
          {item.notes.map((note) => (
            <li key={note}>{note in noteLabels ? t(noteLabels[note]!) : note}</li>
          ))}
        </ul>
      )}

      {item.certificate === undefined ? null : (
        <ul className="flex flex-col gap-1 break-all text-xs text-ink-muted">
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
      <dd className="min-w-0 break-words text-ink">{value}</dd>
    </div>
  );
}
