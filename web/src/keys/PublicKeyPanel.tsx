import { useTranslate } from "../i18n/context";
import { CopyButton } from "../ui/CopyButton";
import { sectionHeading } from "../ui/form";
import { Button } from "../ui/surface";

// The public key of one row, shown under that row with a copy button.
export function PublicKeyPanel({
  relativePath,
  text,
  onClose,
}: {
  relativePath: string;
  text: string;
  onClose: () => void;
}) {
  const t = useTranslate();
  return (
    <section
      aria-labelledby="public-key-heading"
      className="flex flex-col gap-3 rounded-md border border-line bg-surface-subtle p-3 sm:p-4"
    >
      <h3 id="public-key-heading" className={sectionHeading}>
        {t("keys.publicKeyHeading", { path: relativePath })}
      </h3>
      <pre aria-label={t("keys.publicKeyLabel")} className="overflow-x-auto rounded-md bg-canvas p-4 text-xs">
        {text}
      </pre>
      <div className="flex flex-wrap gap-2">
        <CopyButton value={text} label="copy.publicKey" />
        <Button onClick={onClose}>{t("keys.close")}</Button>
      </div>
    </section>
  );
}
