import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { hintText } from "../ui/form";
import type { KeyItem } from "./api";

// KeyTrashConfirmation asks before a key goes to the trash, as a dialog like the permanent delete: what
// goes with it (its public key and certificates) and which hosts still name it. error is why the last
// try failed, shown in the dialog that is still open.
export function KeyTrashConfirmation({
  item,
  members,
  error,
  onConfirm,
  onCancel,
}: {
  item: KeyItem | null;
  members: KeyItem[];
  error: string;
  onConfirm: (id: string) => Promise<void>;
  onCancel: () => void;
}) {
  const t = useTranslate();
  if (item === null) return null;
  return (
    <ConfirmDialog
      id="trash-confirm-heading"
      heading={t("keys.trashConfirmHeading", { path: item.relativePath })}
      body={
        <div className="flex flex-col gap-3">
          <p className="text-sm text-ink">{t("keys.trashExplain")}</p>
          <ul className="flex flex-col gap-0.5 font-mono text-xs text-ink-muted">
            {members.map((member) => (
              <li key={member.id}>{member.relativePath}</li>
            ))}
          </ul>
          {item.references.length === 0 ? (
            <p className={hintText}>{t("keys.trashNoReferences")}</p>
          ) : (
            <>
              <p className="text-sm text-notice-ink">{t("keys.trashReferences", { count: item.references.length })}</p>
              <ul className="flex flex-col gap-0.5 font-mono text-xs text-notice-ink">
                {item.references.map((reference, index) => (
                  <li key={`${reference.configPath}-${reference.line}-${index}`}>
                    {`${reference.hostPatterns.join(" ")} · ${reference.configPath}:${reference.line}`}
                  </li>
                ))}
              </ul>
            </>
          )}
          <p className={hintText}>{t("keys.trashIsRecoverable")}</p>
        </div>
      }
      confirmLabel={t("keys.trashConfirm")}
      cancelLabel={t("keys.trashCancel")}
      onConfirm={() => onConfirm(item.id)}
      onCancel={onCancel}
      error={error}
    />
  );
}
