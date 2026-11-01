import { useEffect, useMemo, useRef, useState, type SyntheticEvent } from "react";
import { useTranslate } from "../i18n/context";
import { isChangeEvent, type FieldSaved } from "../ui/fieldSaved";
import { useSettings } from "./settingsStore";

// savedShownMs is how long "Saved" stays: long enough to read, short enough not to stay on screen as the result
// of something already done.
const savedShownMs = 2_000;

// useSettingsSaved says where a settings page shows "Saved" once a change is saved: under the row (FieldRow) the
// last change was made in, or beside the page's name when it was made outside the rows (the machines' table, the
// preset picker). The settings pages save every change at once, with no Save button, so this is how they all say
// so. forgetRow goes on the whole page, before the rows: a change outside every row then leaves no row noted.
export function useSettingsSaved() {
  const t = useTranslate();
  const { saved } = useSettings();
  const changedRow = useRef<string | null>(null);
  const seen = useRef(saved);
  const [shown, setShown] = useState<{ row: string | null } | null>(null);
  useEffect(() => {
    if (saved === seen.current) return;
    seen.current = saved;
    setShown({ row: changedRow.current });
    const timer = window.setTimeout(() => setShown(null), savedShownMs);
    return () => window.clearTimeout(timer);
  }, [saved]);
  const text = t("settings.saved");
  const fieldSaved = useMemo<FieldSaved>(() => ({
    noteChange: (row) => { changedRow.current = row; },
    savedRow: shown?.row ?? null,
    savedText: text,
  }), [shown, text]);
  return {
    fieldSaved,
    shown: shown !== null,
    besidePageName: shown !== null && shown.row === null,
    text,
    forgetRow: (event: SyntheticEvent) => {
      if (isChangeEvent(event)) changedRow.current = null;
    },
  };
}

