import { useId } from "react";
import { useLanguage } from "../i18n/context";

// PasswordlessSwitch is the choice to use the vault without a master password, drawn as a switch in a
// tinted row as in the design. The checkbox stays a checkbox for assistive technology. Its explanation
// shows once it is on, where the password fields were.
export function PasswordlessSwitch({ checked, onChange }: { checked: boolean; onChange: (checked: boolean) => void }) {
  const { t } = useLanguage();
  const hintId = useId();
  return (
    <div className="rounded-[10px] bg-well px-3 py-2.5">
      <label className="flex cursor-pointer items-center gap-2.5 text-[13px] text-ink">
        {/* The checkbox lies transparent over the drawn switch, so pressing the switch presses the checkbox. */}
        <span className="relative h-[18px] w-[30px] shrink-0">
          <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)}
            aria-describedby={checked ? hintId : undefined} className="peer absolute inset-0 z-10 m-0 cursor-pointer opacity-0" />
          <span aria-hidden="true" className="absolute inset-0 rounded-full bg-ink/20 transition-colors after:absolute after:left-0.5 after:top-0.5 after:size-3.5 after:rounded-full after:bg-sidebar after:transition-transform peer-checked:bg-accent peer-checked:after:translate-x-3 peer-focus-visible:ring-[3px] peer-focus-visible:ring-accent/30" />
        </span>
        {t("lock.withoutPassword")}
      </label>
      {checked ? <p id={hintId} className="mt-1 pl-10 text-[11.5px] leading-[17px] text-ink-faint">{t("lock.withoutPasswordHint")}</p> : null}
    </div>
  );
}
