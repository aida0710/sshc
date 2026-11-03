import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { maxKeyBarPresetNameLength, maxKeyBarPresets } from "../rules/keyBar.generated";
import { defaultPresetId } from "../settings/presetLibrary";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { sizedControl } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { PageSection } from "../ui/page";
import { newIdentifier } from "../ui/randomIdentifier";
import { defaultKeyBarLayout, type KeyBarLayout } from "./keyBarLayout";
import { builtInKeyBarPresets, saveKeyBarLayout, selectKeyBarPreset, updateKeyBarPresets, useKeyBarPresets } from "./keyBarPresets";

type Naming = "copy" | "rename";

// KeyBarPresetSection chooses the key bar preset a settings preset uses, and holds the actions that copy,
// rename, reset and delete key bar presets. The default layout and the built-in presets are not saved presets: they
// can be copied, not renamed, reset or deleted.
export function KeyBarPresetSection({ presetId, layout, onError }: {
  presetId?: string | undefined;
  layout: KeyBarLayout;
  onError: (message: string) => void;
}) {
  const t = useTranslate();
  const library = useKeyBarPresets(presetId);
  const [naming, setNaming] = useState<Naming | null>(null);
  const [deleting, setDeleting] = useState(false);
  const unavailable = library.loading || library.busy || library.error;
  const chosen = library.presets.find((preset) => preset.id === library.selected);
  const chosenName = chosen?.name ?? builtInKeyBarPresets.find((preset) => preset.id === library.selected)?.name ?? t("keyBar.defaultPreset");

  async function attempt(action: () => Promise<void>) {
    onError("");
    try { await action(); } catch { onError(t("keyBar.saveFailed")); }
  }

  async function name(value: string) {
    setNaming(null);
    await attempt(async () => {
      if (naming === "rename") {
        await updateKeyBarPresets(library.presets.map((preset) => preset.id === library.selected ? { ...preset, name: value } : preset), library.selected, presetId);
        return;
      }
      const id = newIdentifier();
      await updateKeyBarPresets([...library.presets, { id, name: value, ...layout }], id, presetId);
    });
  }

  const actions: ActionMenuItem[] = [
    { label: t("keyBar.duplicate"), onSelect: () => setNaming("copy"), disabled: unavailable || library.presets.length >= maxKeyBarPresets },
    { label: t("keyBar.rename"), onSelect: () => setNaming("rename"), disabled: unavailable || chosen === undefined },
    { label: t("keyBar.reset"), onSelect: () => void attempt(() => saveKeyBarLayout(defaultKeyBarLayout, chosenName, presetId)), disabled: unavailable || chosen === undefined },
    { label: t("keyBar.delete"), onSelect: () => setDeleting(true), tone: "danger", disabled: unavailable || chosen === undefined },
  ];

  return (
    <PageSection heading={t("keyBar.presetsHeading")} {...(chosen === undefined ? { hint: t("keyBar.unsavedPresetHint", { name: chosenName }) } : {})}>
      <div className="flex min-w-0 items-center gap-2">
        <select aria-label={t("keyBar.preset")} className={sizedControl("w-full min-w-0 md:max-w-xs")} value={library.selected} disabled={unavailable}
          onChange={(event) => void attempt(() => selectKeyBarPreset(event.target.value, presetId))}>
          <option value={defaultPresetId}>{t("keyBar.defaultPreset")}</option>
          {builtInKeyBarPresets.map((preset) => <option key={preset.id} value={preset.id}>{preset.name}</option>)}
          {library.presets.map((preset) => <option key={preset.id} value={preset.id}>{preset.name}</option>)}
        </select>
        <ActionMenu label={t("keyBar.presetActions", { name: chosenName })} items={actions}
          triggerClassName="flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink md:size-7" />
      </div>
      {naming === null ? null : (
        <InputDialog
          id="key-bar-preset-name"
          heading={naming === "copy" ? t("keyBar.duplicateHeading", { name: chosenName }) : t("keyBar.renameHeading")}
          label={t("keyBar.presetName")}
          initialValue={naming === "rename" ? chosenName : ""}
          submitLabel={naming === "copy" ? t("keyBar.duplicate") : t("keyBar.rename")}
          cancelLabel={t("keyBar.cancel")}
          validate={(value) => value === "" ? t("keyBar.presetNameRequired")
            : [...value].length > maxKeyBarPresetNameLength ? t("keyBar.presetNameTooLong", { max: String(maxKeyBarPresetNameLength) }) : ""}
          onSubmit={name}
          onCancel={() => setNaming(null)}
        />
      )}
      {deleting && chosen !== undefined ? (
        <ConfirmDialog
          id="key-bar-preset-delete"
          heading={t("keyBar.confirmDelete")}
          body={<p className="text-sm text-ink-muted">{t("keyBar.deleteBody", { name: chosen.name })}</p>}
          confirmLabel={t("keyBar.deleteEverywhere")}
          cancelLabel={t("keyBar.cancel")}
          onCancel={() => setDeleting(false)}
          onConfirm={() => {
            setDeleting(false);
            void attempt(() => updateKeyBarPresets(library.presets.filter((preset) => preset.id !== chosen.id), defaultPresetId, presetId));
          }}
        />
      ) : null}
    </PageSection>
  );
}
