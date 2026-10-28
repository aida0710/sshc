import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Notice } from "../ui/surface";
import { DraftSaveBar } from "./DraftSaveBar";

type HostBlockTextProps = {
  raw: string;
  // dirty は、文が保存した中身と違うあいだ（保存し終えたら、読み直す前でも false）。
  dirty: boolean;
  onChange: (raw: string) => void;
  onBack: () => void;
  // blockedByFields は、ディレクティブの下書きがあって文を編集できないとき。
  blockedByFields: boolean;
  disabled: boolean;
  saving: boolean;
  onDiscard: () => void;
  onSave: () => void;
};

// HostBlockText は、Host ブロックの文をそのまま編集する欄である。見出しの「…」の［テキストで編集］で開く。
// コメント、空行、sshc が知らないディレクティブも、書いたとおりに書き戻す。
export function HostBlockText({ raw, dirty, onChange, onBack, blockedByFields, disabled, saving, onDiscard, onSave }: HostBlockTextProps) {
  const t = useTranslate();
  return (
    <section aria-label={t("conn.editAsText")} className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <button type="button" onClick={onBack} className="inline-flex items-center gap-1 rounded-md px-1.5 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink">
          <Icon name="chevronLeft" className="size-3.5" />
          {t("advanced.backToSettings")}
        </button>
      </div>
      {blockedByFields ? <Notice>{t("conn.advancedFieldsBlockRaw")}</Notice> : null}
      <label htmlFor="block-raw" className="text-xs text-ink-muted">{t("host.blockText")}</label>
      <textarea
        id="block-raw"
        value={raw}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        rows={16}
        spellCheck={false}
        className="min-h-80 rounded-lg border border-control-line bg-tree p-4 font-mono text-xs leading-5 text-ink focus:border-accent focus:outline-none"
      />
      {dirty ? (
        <DraftSaveBar saveLabel={t("host.saveBlock")} saving={saving} saveDisabled={disabled} discardDisabled={saving} onDiscard={onDiscard} onSave={onSave} />
      ) : null}
    </section>
  );
}
