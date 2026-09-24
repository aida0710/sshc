import { useCallback, useEffect, useState } from "react";
import type { HostDetail, HostMetadata } from "../api/config";
import type { VPNProfile } from "../api/vpn";
import { Field, control, fieldLabel, hintText } from "../ui/form";
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
import { NoticeList } from "./SavePreview";
import { AppearancePicker } from "../terminal/AppearancePicker";
import { BackgroundPicker } from "../terminal/BackgroundPicker";
import { chooseAppearance } from "../terminal/appearance";
import { fonts } from "../terminal/fonts";
import { operatingSystems, OperatingSystemIcon } from "../ui/OperatingSystemIcon";
import { palettes } from "../terminal/palettes";

function inherited(detail: HostDetail) {
  const own = detail.form.entry.file.path ?? detail.form.entry.file.absolute;
  return detail.effective.entries.filter((entry) => (entry.source.path ?? entry.source.absolute) !== own);
}

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

// HostInspector は、接続エディタのsshcタブである。sshcだけが使う接続ごとの設定を下書きとして
// 編集し、保存を押したときにまとめて保存する。
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
  const [saving, setSaving] = useState(false);
  const [saveFailed, setSaveFailed] = useState(false);
  const dirty = !sameHostMetadata(draft, saved);
  const notices = [...(detail.form.notices ?? []), ...(detail.effective.notices ?? [])];
  const fromElsewhere = inherited(detail);

  const discard = useCallback(() => {
    setDraft(saved);
    setTagsText(formatTags(saved.tags));
    setSaveFailed(false);
  }, [saved]);

  // 別の接続を開いたとき、または保存済みの値が変わったとき（保存したあとを含む）は、
  // 下書きを保存済みの値からやり直す。同じ値を読み直しただけでは下書きを消さない。
  const resetKey = `${identityKey(detail.form.entry.identity)}\u0000${canonicalHostMetadata(saved)}`;
  useEffect(() => {
    discard();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  useEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange]);

  useEffect(() => {
    onDiscardReady?.(discard);
    return () => onDiscardReady?.(null);
  }, [discard, onDiscardReady]);

  function edit(next: HostMetadata) {
    setDraft(next);
    setSaveFailed(false);
  }

  async function save() {
    if (!dirty || saving || disabled) return;
    setSaving(true);
    setSaveFailed(false);
    try {
      await onSave(draft);
    } catch {
      setSaveFailed(true);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">

      <section className="flex flex-col gap-3">
        <h3 className="text-sm font-semibold text-ink">{t("inspector.appOnly")}</h3>

        <fieldset disabled={disabled || saving} className="contents">
        <div className="flex flex-col gap-4 rounded-md bg-tree p-4">
        <div className="flex flex-col gap-2">
          <Field label={t("host.colour")}>
            <input
              type="color"
              value={
                draft.colour === undefined || draft.colour === ""
                  ? "#8e8e93" /* palette-exempt: ネイティブコントロール自身の中立色 */
                  : draft.colour
              }
              onChange={(event) => edit({ ...draft, colour: event.target.value })}
              className="h-8 w-14 rounded border border-control-line bg-control"
            />
          </Field>
          {draft.colour === undefined || draft.colour === "" ? null : (
            <Button className="self-start" onClick={() => edit({ ...draft, colour: "" })}>
              {t("host.clearColour")}
            </Button>
          )}
        </div>

        <Field label={t("host.os")} hint={t("host.osHint")} interactiveChildren>
          <div className="flex items-center gap-2">
            <OperatingSystemIcon os={draft.os || draft.detectedOS || ""} />
            <select aria-label={t("host.os")} value={draft.os ?? ""}
              onChange={(event) => edit({ ...draft, os: event.target.value as NonNullable<HostMetadata["os"]> })}
              className={control}>
              <option value="">{t("host.osAutomatic")}</option>
              {operatingSystems.map(([value, label]) => <option key={value} value={value}>{value === "server" ? t("host.osGeneric") : label}</option>)}
            </select>
          </div>
        </Field>

        <Field label={t("connection.paletteLabel")} hint={t("connection.paletteHint")}>
          <AppearancePicker
            choices={palettes}
            value={draft.appearance?.palette ?? ""}
            onChange={(chosen) => edit(chooseAppearance(draft, { palette: chosen }))}
            unchosen={t("terminal.paletteFollowsOverall")}
          />
        </Field>

        <Field label={t("connection.fontLabel")} hint={t("connection.fontHint")}>
          <AppearancePicker
            choices={fonts}
            value={draft.appearance?.font ?? ""}
            onChange={(chosen) => edit(chooseAppearance(draft, { font: chosen }))}
            unchosen={t("terminal.fontFollowsOverall")}
          />
        </Field>

        <Field label={t("connection.backgroundLabel")} hint={t("connection.backgroundHint")} interactiveChildren>
          <BackgroundPicker
            value={draft.appearance?.background ?? ""}
            onChange={(chosen) => edit(chooseAppearance(draft, { background: chosen }))}
            tint={draft.appearance?.backgroundTint}
            onTintChange={(chosen) => edit(chooseAppearance(draft, { backgroundTint: chosen }))}
            unchosen={t("terminal.backgroundFollowsOverall")}
          />
        </Field>

        <Field label={t("connection.encodingLabel")} hint={t("connection.encodingHint")}>
          <select
            value={draft.encoding ?? ""}
            onChange={(event) =>
              edit(withOptionalChoice(draft, "encoding", event.target.value as NonNullable<HostMetadata["encoding"]> | ""))}
            className={control}
          >
            <option value="">{t("connection.encodingUTF8")}</option>
            <option value="shift_jis">{t("connection.encodingShiftJIS")}</option>
            <option value="euc-jp">{t("connection.encodingEUCJP")}</option>
            <option value="iso-2022-jp">{t("connection.encodingISO2022JP")}</option>
          </select>
        </Field>

        <Field label={t("connection.osc52Label")} hint={t("connection.osc52Hint")}>
          <select
            value={draft.osc52 ?? ""}
            onChange={(event) =>
              edit(withOptionalChoice(draft, "osc52", event.target.value as NonNullable<HostMetadata["osc52"]> | ""))}
            className={control}
          >
            <option value="">{t("connection.osc52Inherit")}</option>
            <option value="allow">{t("connection.osc52Allow")}</option>
            <option value="deny">{t("connection.osc52Deny")}</option>
          </select>
        </Field>

        <HostVPNProfileField
          detail={detail}
          value={draft.vpn ?? ""}
          profiles={vpnProfiles}
          onChange={(profile) => edit(withOptionalChoice(draft, "vpn", profile))}
        />

        <Field label={t("host.tags")}>
          <input
            value={tagsText}
            onChange={(event) => {
              setTagsText(event.target.value);
              edit({ ...draft, tags: parseTags(event.target.value) });
            }}
            className={control}
          />
        </Field>

        <Field label={t("host.displayOrder")}>
          <input
            type="number"
            value={String(draft.order ?? 0)}
            onChange={(event) => edit({ ...draft, order: Number(event.target.value) || 0 })}
            className={control}
          />
        </Field>
        </div>
        </fieldset>

        {saveFailed ? <Notice tone="danger">{t("inspector.hostSaveFailed")}</Notice> : null}
        {dirty ? <DraftSaveBar
          saveLabel={t("inspector.hostSave")}
          saving={saving}
          saveDisabled={disabled || saving}
          discardDisabled={saving}
          onDiscard={discard}
          onSave={() => void save()}
        /> : null}
      </section>

      <section className="flex flex-col gap-2">
        <h3 className={fieldLabel}>{t("inspector.notices")}</h3>
        {notices.length === 0 ? (
          <p className={hintText}>{t("inspector.noNotices")}</p>
        ) : (
          <NoticeList notices={notices} />
        )}
      </section>

      <section className="flex flex-col gap-2">
        <h3 className={fieldLabel}>{t("inspector.inherited")}</h3>
        {fromElsewhere.length === 0 ? (
          <p className={hintText}>{t("inspector.noInherited")}</p>
        ) : (
          <ul className="overflow-hidden rounded-lg bg-tree">
            {fromElsewhere.map((entry, index) => (
              <li key={`${entry.keyword}-${index}`} className="border-t border-hairline px-3 py-2 font-mono text-xs text-ink-muted first:border-t-0">
                {`${entry.keyword} ${entry.values.join(" ")} · ${
                  entry.source.path ?? entry.source.absolute ?? ""
                }:${entry.source.line ?? 0}`}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
