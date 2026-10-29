import { useState } from "react";
import { Button } from "../ui/surface";
import { sizedControl } from "../ui/form";
import type { HostEntry, HostMetadata, Metadata } from "../api/config";
import { useTranslate, type Translate } from "../i18n/context";
import { identityKey } from "./connectionBrowser";

type OrphanPanelProps = {
  metadata: Metadata;
  hosts: HostEntry[];
  // onSave は、orphan の entry 1 件（base）を next に置き換える。next が null なら消す。
  onSave: (base: HostMetadata, next: HostMetadata | null) => void;
};

function describe(entry: HostMetadata, t: Translate): string {
  const parts: string[] = [];
  if (entry.tags !== undefined && entry.tags.length > 0) parts.push(t("orphan.tags", { tags: entry.tags.join(", ") }));
  if (entry.colour !== undefined && entry.colour !== "") parts.push(t("orphan.colour", { colour: entry.colour }));
  return parts.length === 0 ? t("orphan.noSettings") : parts.join(" · ");
}

export function OrphanPanel({ metadata, hosts, onSave }: OrphanPanelProps) {
  const t = useTranslate();
  const [targets, setTargets] = useState<Record<string, string>>({});
  const [error, setError] = useState("");

  const entries = metadata.hosts ?? [];
  const orphans = entries.filter((entry) => entry.orphan === true);
  if (orphans.length === 0) return null;

  const occupied = new Set(
    entries
      .filter((entry) => entry.orphan !== true)
      .map((entry) => identityKey(entry.identity)),
  );

  // The select's option values are identity keys, so the chosen key maps back
  // to its identity without taking the key apart again.
  const candidateIdentities = new Map(
    hosts
      .filter((host) => host.identity.alias !== "")
      .map((host) => [identityKey(host.identity), host.identity] as const),
  );

  function withoutOrphanFlag(entry: HostMetadata): HostMetadata {
    const { orphan: _orphan, ...rest } = entry;
    return rest;
  }

  function reassociate(entry: HostMetadata) {
    const key = identityKey(entry.identity);
    const chosen = targets[key] ?? "";
    if (chosen === "") {
      setError(t("orphan.chooseTarget"));
      return;
    }
    const target = candidateIdentities.get(chosen);
    if (target === undefined) return;
    if (occupied.has(chosen)) {
      setError(t("orphan.occupied", { alias: target.alias }));
      return;
    }
    setError("");
    onSave(entry, { ...withoutOrphanFlag(entry), identity: { path: target.path, alias: target.alias } });
  }

  function discard(entry: HostMetadata) {
    setError("");
    onSave(entry, null);
  }

  return (
    <section aria-labelledby="orphan-heading" className="flex flex-col gap-3 rounded-xl border border-notice-line bg-notice p-4">
      <h3 id="orphan-heading" className="text-sm font-medium text-notice-ink">
        {t("orphan.heading")}
      </h3>
      <p className="text-xs text-ink-muted">{t("orphan.explain")}</p>
      {error === "" ? null : (
        <p role="alert" className="text-xs text-danger">
          {error}
        </p>
      )}
      <ul className="overflow-hidden rounded-lg bg-card">
        {orphans.map((entry) => {
          const key = identityKey(entry.identity);
          return (
            <li key={key} className="flex flex-col gap-2 border-t border-hairline p-3 first:border-t-0">
              <p className="text-sm text-ink">
                {t("orphan.entry", { alias: entry.identity.alias, path: entry.identity.path })}
              </p>
              <p className="text-xs text-ink-muted">{describe(entry, t)}</p>
              <div className="flex flex-wrap items-center gap-2">
                <label htmlFor={`orphan-target-${key}`} className="sr-only">
                  {t("orphan.reassociateWith", { alias: entry.identity.alias })}
                </label>
                <select
                  id={`orphan-target-${key}`}
                  value={targets[key] ?? ""}
                  onChange={(event) => setTargets({ ...targets, [key]: event.target.value })}
                  className={sizedControl("min-w-48")}
                >
                  <option value="">{t("orphan.reassociatePlaceholder")}</option>
                  {Array.from(candidateIdentities, ([candidateKey, identity]) => (
                    <option key={candidateKey} value={candidateKey}>
                      {t("orphan.entry", { alias: identity.alias, path: identity.path })}
                    </option>
                  ))}
                </select>
                <Button
                  onClick={() => reassociate(entry)}
                >
                  {t("orphan.reassociate", { alias: entry.identity.alias })}
                </Button>
                <Button kind="danger" onClick={() => discard(entry)}>
                  {t("orphan.discard", { alias: entry.identity.alias })}
                </Button>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
