import { useCallback, useMemo, useRef, useState, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { useDraftDiscardConfirmation } from "../routing/useDraftDiscardConfirmation";
import type { NavigateLocationOptions, NavigationBlocker } from "../routing/useSectionRoute";
import { DiscardDraftDialog } from "../ui/DiscardDraftDialog";
import { useToast } from "../ui/useToast";
import { SettingsDraftContext, type DescribeFailure, type DraftPart, type DraftProblem, type SettingsDraft } from "./settingsDraftContext";
import { changedParts, editGlobal, editMachine, editPreset, noEdits, withEdits, withoutPart, type SettingsEdits, type SettingsPartChange } from "./settingsEdits";
import { updateGlobalSettings, updateMachine, updatePreset, useSettings } from "./settingsStore";

function savePart(part: SettingsPartChange): Promise<void> {
  if (part.kind === "global") return updateGlobalSettings(part.change);
  if (part.kind === "preset") return updatePreset(part.id, part.change);
  return updateMachine(part.id, part.change);
}

type SettingsDraftProviderProps = {
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onNavigateLocation?: ((url: string, options?: NavigateLocationOptions) => void) | undefined;
  children: ReactNode;
};

// SettingsDraftProvider は、設定の画面の保存していない変更を持ち、［保存］でまとめて書く。保存は宛先（全体の設定、
// プリセット、マシン、ショートカットなど）ごとに分かれ、書けた宛先の変更だけを下書きから除く。保存していない変更が
// あるあいだは、ほかのページや画面へ移る前に、破棄してよいかを確かめる。
export function SettingsDraftProvider({ onNavigationBlockerChange, onNavigateLocation, children }: SettingsDraftProviderProps) {
  const t = useTranslate();
  const showToast = useToast();
  const { document: stored, loadFailed } = useSettings();
  const [edits, setEdits] = useState<SettingsEdits>(noEdits);
  const [problems, setProblems] = useState<ReadonlyMap<string, DraftProblem>>(new Map());
  const [parts, setParts] = useState<ReadonlyMap<string, DraftPart>>(new Map());
  const [discarded, setDiscarded] = useState(0);
  const [saving, setSaving] = useState(false);
  const [failure, setFailure] = useState("");
  const [savedCount, setSavedCount] = useState(0);
  const failureDescribers = useRef(new Set<DescribeFailure>());

  const changed = useMemo(() => stored === null ? [] : changedParts(stored, edits), [stored, edits]);
  const document = useMemo(() => stored === null ? null : withEdits(stored, edits), [stored, edits]);
  const inputInvalid = problems.size > 0;
  const problemOutOfSight = [...problems.values()].find((candidate) => candidate.outOfSight)?.text ?? "";
  const dirty = changed.length > 0 || inputInvalid || [...parts.values()].some((part) => part.dirty);

  const discard = useCallback(() => {
    setEdits(noEdits);
    setFailure("");
    setDiscarded((count) => count + 1);
    parts.forEach((part) => part.discard());
  }, [parts]);

  const draftDiscard = useDraftDiscardConfirmation({ dirty, discard, onNavigationBlockerChange, onNavigateLocation });

  async function save() {
    if (!dirty || inputInvalid || saving) return;
    setSaving(true);
    setFailure("");
    const savedEdits = edits;
    try {
      // ショートカットなどの宛先は、設定で選んだプリセットに書くので、設定のあとに保存する。
      for (const part of changed) {
        await savePart(part);
        setEdits((current) => withoutPart(current, savedEdits, part));
      }
      for (const part of parts.values()) {
        if (part.dirty) await part.save();
      }
      setSavedCount((count) => count + 1);
      showToast(t("draft.saved"), { tone: "success" });
    } catch (error) {
      setFailure(describeFailure(error));
    } finally {
      setSaving(false);
    }
  }

  const latestSave = useRef(save);
  latestSave.current = save;

  function describeFailure(error: unknown): string {
    for (const describe of failureDescribers.current) {
      const text = describe(error);
      if (text !== null) return text;
    }
    return t("settings.saveFailed");
  }

  // 変えたら、前の失敗の文はもう今の下書きを指さない。
  const edit = useCallback((change: (current: SettingsEdits) => SettingsEdits) => {
    setFailure("");
    setEdits(change);
  }, []);
  const { confirmBefore: confirmBeforeDiscarding } = draftDiscard;
  const setProblem = useCallback((id: string, found: DraftProblem | null) => setProblems((current) => {
    if (found === null && !current.has(id)) return current;
    const next = new Map(current);
    if (found === null) next.delete(id);
    else next.set(id, found);
    return next;
  }), []);
  const setPart = useCallback((id: string, part: DraftPart | null) => setParts((current) => {
    const next = new Map(current);
    if (part === null) next.delete(id);
    else next.set(id, part);
    return next;
  }), []);
  const addDescribeFailure = useCallback((describe: DescribeFailure) => {
    failureDescribers.current.add(describe);
    return () => { failureDescribers.current.delete(describe); };
  }, []);

  const value = useMemo<SettingsDraft>(() => ({
    document,
    loadFailed,
    editGlobal: (field, change) => edit((current) => editGlobal(current, field, change)),
    editPreset: (id, field, change) => edit((current) => editPreset(current, id, field, change)),
    editMachine: (id, field, change) => edit((current) => editMachine(current, id, field, change)),
    discarded,
    confirmBeforeDiscarding,
    dirty,
    inputInvalid,
    problemOutOfSight,
    failure,
    saving,
    savedCount,
    save: () => void latestSave.current(),
    discard,
    setProblem,
    setPart,
    addDescribeFailure,
  }), [document, loadFailed, edit, discarded, confirmBeforeDiscarding, dirty, inputInvalid, problemOutOfSight, failure, saving, savedCount, discard, setProblem, setPart, addDescribeFailure]);

  return (
    <SettingsDraftContext.Provider value={value}>
      {children}
      {draftDiscard.confirming ? (
        <DiscardDraftDialog id="settings-discard" onConfirm={draftDiscard.confirmDiscard} onCancel={draftDiscard.keepEditing} />
      ) : null}
    </SettingsDraftContext.Provider>
  );
}
