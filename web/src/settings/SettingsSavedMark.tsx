import { useEffect, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { useSettings } from "./settingsStore";

// savedShownMs is how long "Saved" stays by the page's name: long enough to read, short enough not to stay on
// screen as the result of something already done.
const savedShownMs = 2_000;

// SettingsSavedMark says, for a moment beside the page's name, that a change was saved. The settings pages save
// every change at once, with no Save button, so this is where they all say so.
export function SettingsSavedMark() {
  const t = useTranslate();
  const { saved } = useSettings();
  const [shown, setShown] = useState(false);
  const seen = useRef(saved);
  useEffect(() => {
    if (saved === seen.current) return;
    seen.current = saved;
    setShown(true);
    const timer = window.setTimeout(() => setShown(false), savedShownMs);
    return () => window.clearTimeout(timer);
  }, [saved]);
  return (
    <span role="status" className="text-xs font-normal text-ink-faint">
      {shown ? t("settings.saved") : ""}
    </span>
  );
}
