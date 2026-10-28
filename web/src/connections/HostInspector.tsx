import { useCallback, useEffect, useState } from "react";
import type { HostDetail, HostMetadata } from "../api/config";
import type { VPNProfile } from "../api/vpn";
import { control } from "../ui/form";
import { FieldRow } from "../ui/FieldRow";
import { Button, Notice } from "../ui/surface";
import { useTranslate } from "../i18n/context";
import { identityKey } from "./connectionBrowser";
import { DraftSaveBar } from "./DraftSaveBar";
import {
  canonicalHostMetadata,
  formatTags,
  parseTags,
  sameHostMetadata,
  withOptionalChoice,
} from "./hostMetadataDraft";
import { HostVPNProfileField } from "./HostVPNProfileField";
import { EditorSection } from "./EditorSection";
import { useDraftSave } from "./useDraftSave";
import { useReportDirty } from "./useReportDirty";
import { AppearancePicker } from "../terminal/AppearancePicker";
import { BackgroundPicker } from "../terminal/BackgroundPicker";
import { chooseAppearance } from "../terminal/appearance";
import { fonts } from "../terminal/fonts";
import { operatingSystems, OperatingSystemIcon } from "../ui/OperatingSystemIcon";
import { palettes } from "../terminal/palettes";

type HostInspectorProps = {
  detail: HostDetail;
  // onSave は、下書きを保存する。保存できなかったときは reject する。
  onSave: (metadata: HostMetadata) => Promise<void>;
  // vpnProfiles は、この接続を通せるVPNプロファイルである。
  vpnProfiles?: VPNProfile[] | undefined;
  onDirtyChange?: ((dirty: boolean) => void) | undefined;
  onDiscardReady?: ((discard: (() => void) | null) => void) | undefined;
  disabled?: boolean | undefined;
};

// HostInspector は、接続エディタの「表示とターミナル」のタブである。sshc だけが使う接続ごとの設定
// （一覧での見え方、ターミナル、VPN）を下書きとして編集し、保存を押したときにまとめて保存する。
export function HostInspector({
  detail,
  onSave,
  vpnProfiles = [],
  onDirtyChange,
  onDiscardReady,
  disabled = false,
}: HostInspectorProps) {
  const t = useTranslate();
  const saved = detail.metadata;
  const [draft, setDraft] = useState<HostMetadata>(saved);
  // tagsText は、タグの入力欄の文字列である。入力途中のカンマや空白を消さないよう、
  // 下書きのタグとは別に持つ。
  const [tagsText, setTagsText] = useState(() => formatTags(saved.tags));
  const { saving, written, saveFailed, save: saveDraft } = useDraftSave({ draft, saved });
  const dirty = !written && !sameHostMetadata(draft, saved);

  const discard = useCallback(() => {
    setDraft(saved);
    setTagsText(formatTags(saved.tags));
  }, [saved]);

  // 別の接続を開いたとき、または保存済みの値が変わったとき（保存したあとを含む）は、
  // 下書きを保存済みの値からやり直す。同じ値を読み直しただけでは下書きを消さない。
  const resetKey = `${identityKey(detail.form.entry.identity)}\u0000${canonicalHostMetadata(saved)}`;
  useEffect(() => {
    discard();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  useReportDirty(dirty, onDirtyChange);

  useEffect(() => {
    onDiscardReady?.(discard);
    return () => onDiscardReady?.(null);
  }, [discard, onDiscardReady]);

  function save() {
    if (!dirty || saving || disabled) return;
    void saveDraft(() => onSave(draft));
  }

  return (
    <div className="flex flex-col">
      <fieldset disabled={disabled || saving} className="flex min-w-0 flex-col">
        <EditorSection title={t("inspector.lookHeading")}>
          <FieldRow
            density="editor"
            label={t("host.colour")}
            actions={draft.colour === undefined || draft.colour === "" ? undefined : (
              <Button small onClick={() => setDraft({ ...draft, colour: "" })}>
                {t("host.clearColour")}
              </Button>
            )}
          >
            <input
              type="color"
              value={
                draft.colour === undefined || draft.colour === ""
                  ? "#8e8e93" /* palette-exempt: ネイティブコントロール自身の中立色 */
                  : draft.colour
              }
              onChange={(event) => setDraft({ ...draft, colour: event.target.value })}
              className="h-8 w-14 rounded border border-control-line bg-control"
            />
          </FieldRow>

          <FieldRow density="editor" label={t("host.os")} interactiveChildren>
            <div className="flex items-center gap-2">
              <OperatingSystemIcon os={draft.os || draft.detectedOS || ""} />
              <select aria-label={t("host.os")} value={draft.os ?? ""}
                onChange={(event) => setDraft({ ...draft, os: event.target.value as NonNullable<HostMetadata["os"]> })}
                className={control}>
                <option value="">{t("host.osAutomatic")}</option>
                {operatingSystems.map(([value, label]) => <option key={value} value={value}>{value === "server" ? t("host.osGeneric") : label}</option>)}
              </select>
            </div>
          </FieldRow>

          <FieldRow density="editor" label={t("host.tags")}>
            <input
              value={tagsText}
              onChange={(event) => {
                setTagsText(event.target.value);
                setDraft({ ...draft, tags: parseTags(event.target.value) });
              }}
              className={control}
            />
          </FieldRow>

          <FieldRow density="editor" label={t("host.displayOrderLabel")} hint={t("host.displayOrderHint")}>
            <input
              type="number"
              value={String(draft.order ?? 0)}
              onChange={(event) => setDraft({ ...draft, order: Number(event.target.value) || 0 })}
              className={`${control} w-28`}
            />
          </FieldRow>
        </EditorSection>

        <EditorSection title={t("inspector.terminalHeading")} meta={t("inspector.terminalFollowsApp")}>
          <FieldRow density="editor" label={t("connection.paletteLabel")}>
            <AppearancePicker
              choices={palettes}
              value={draft.appearance?.palette ?? ""}
              onChange={(chosen) => setDraft(chooseAppearance(draft, { palette: chosen }))}
              unchosen={t("terminal.paletteFollowsOverall")}
            />
          </FieldRow>

          <FieldRow density="editor" label={t("connection.fontLabel")}>
            <AppearancePicker
              choices={fonts}
              value={draft.appearance?.font ?? ""}
              onChange={(chosen) => setDraft(chooseAppearance(draft, { font: chosen }))}
              unchosen={t("terminal.fontFollowsOverall")}
            />
          </FieldRow>

          <FieldRow density="editor" label={t("connection.backgroundLabel")} interactiveChildren>
            <BackgroundPicker
              value={draft.appearance?.background ?? ""}
              onChange={(chosen) => setDraft(chooseAppearance(draft, { background: chosen }))}
              tint={draft.appearance?.backgroundTint}
              onTintChange={(chosen) => setDraft(chooseAppearance(draft, { backgroundTint: chosen }))}
              unchosen={t("terminal.backgroundFollowsOverall")}
            />
          </FieldRow>

          <FieldRow density="editor" label={t("connection.encodingLabel")} hint={t("connection.encodingHint")}>
            <select
              value={draft.encoding ?? ""}
              onChange={(event) =>
                setDraft(withOptionalChoice(draft, "encoding", event.target.value as NonNullable<HostMetadata["encoding"]> | ""))}
              className={control}
            >
              <option value="">{t("connection.encodingUTF8")}</option>
              <option value="shift_jis">{t("connection.encodingShiftJIS")}</option>
              <option value="euc-jp">{t("connection.encodingEUCJP")}</option>
              <option value="iso-2022-jp">{t("connection.encodingISO2022JP")}</option>
            </select>
          </FieldRow>

          <FieldRow density="editor" label={t("connection.osc52Label")} hint={t("connection.osc52Hint")}>
            <select
              value={draft.osc52 ?? ""}
              onChange={(event) =>
                setDraft(withOptionalChoice(draft, "osc52", event.target.value as NonNullable<HostMetadata["osc52"]> | ""))}
              className={control}
            >
              <option value="">{t("connection.osc52Inherit")}</option>
              <option value="allow">{t("connection.osc52Allow")}</option>
              <option value="deny">{t("connection.osc52Deny")}</option>
            </select>
          </FieldRow>
        </EditorSection>

        <EditorSection title={t("inspector.vpnHeading")}>
          <HostVPNProfileField
            detail={detail}
            value={draft.vpn ?? ""}
            saved={saved.vpn ?? ""}
            profiles={vpnProfiles}
            onChange={(profile) => setDraft(withOptionalChoice(draft, "vpn", profile))}
          />
        </EditorSection>
      </fieldset>

      {saveFailed ? <Notice tone="danger">{t("inspector.hostSaveFailed")}</Notice> : null}
      {dirty ? <DraftSaveBar
        saveLabel={t("inspector.hostSave")}
        saving={saving}
        saveDisabled={disabled || saving}
        discardDisabled={saving}
        onDiscard={discard}
        onSave={save}
      /> : null}
    </div>
  );
}
