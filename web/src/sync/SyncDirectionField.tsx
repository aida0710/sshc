import type { SyncDirection } from "../api/sync";
import { useTranslate } from "../i18n/context";
import { FieldRow } from "../ui/FieldRow";
import { control } from "../ui/form";
import { syncDirectionHintKeys } from "./syncMessageKeys";

// Which way this machine moves data. Each machine chooses its own, so the field is asked wherever a machine
// is set up: typed in by hand or taken from another machine's exported settings.
export function SyncDirectionField({ direction, onChange }: {
  direction: SyncDirection;
  onChange: (direction: SyncDirection) => void;
}) {
  const t = useTranslate();
  return (
    <FieldRow label={t("sync.direction")} hint={t(syncDirectionHintKeys[direction])}>
      <select
        value={direction}
        onChange={(event) => onChange(event.target.value as SyncDirection)}
        className={control}
      >
        <option value="both">{t("sync.role.main")}</option>
        <option value="pull">{t("sync.role.receive")}</option>
        <optgroup label={t("sync.role.advanced")}>
          <option value="push">{t("sync.role.send")}</option>
        </optgroup>
      </select>
    </FieldRow>
  );
}
