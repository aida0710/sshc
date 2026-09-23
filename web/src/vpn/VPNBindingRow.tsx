import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { Field, hintText, sectionHeading, sizedControl } from "../ui/form";
import { Button } from "../ui/surface";

// この経路を通る接続を見せ、増やしたり外したりする。
export function VPNBindingRow({
  profile,
  connections,
  aliases,
  busy,
  onBind,
  onUnbind,
}: {
  profile: string;
  connections: string[];
  aliases: string[];
  busy: boolean;
  onBind: (alias: string) => void;
  onUnbind: (alias: string) => void;
}) {
  const t = useTranslate();
  const [alias, setAlias] = useState("");
  const available = aliases.filter((name) => !connections.includes(name));
  return (
    <div className="flex flex-col gap-2 border-t border-line pt-3">
      <p className={sectionHeading}>{t("vpn.connections")}</p>
      {connections.length === 0 ? (
        <p className={hintText}>{t("vpn.noConnections")}</p>
      ) : (
        <ul className="flex flex-wrap gap-2">
          {connections.map((name) => (
            <li key={name} className="flex items-center gap-1 rounded bg-surface-subtle px-2 py-1 text-sm">
              <span>{name}</span>
              <button
                type="button"
                aria-label={t("vpn.unbindAction", { alias: name, name: profile })}
                className="text-ink-muted hover:text-ink"
                disabled={busy}
                onClick={() => onUnbind(name)}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex flex-wrap items-end gap-2">
        <Field label={t("vpn.bindLabel")}>
          <select
            className={sizedControl("medium")}
            value={alias}
            disabled={busy || available.length === 0}
            onChange={(event) => setAlias(event.target.value)}
          >
            <option value="">{t("vpn.bindChoose")}</option>
            {available.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        </Field>
        <Button
          disabled={busy || alias === ""}
          onClick={() => {
            onBind(alias);
            setAlias("");
          }}
        >
          {t("vpn.bindAction")}
        </Button>
      </div>
    </div>
  );
}
