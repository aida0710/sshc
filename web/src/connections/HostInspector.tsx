import type { HostDetail, HostMetadata } from "../api/config";
import { Field, control, fieldLabel, hintText } from "../ui/form";
import { Button } from "../ui/surface";
import { useTranslate } from "../i18n/context";
import { NoticeList } from "./SavePreview";
import { AppearancePicker } from "../terminal/AppearancePicker";
import { BackgroundPicker } from "../terminal/BackgroundPicker";
import { chooseAppearance } from "../terminal/appearance";
import { fonts } from "../terminal/fonts";
import { palettes } from "../terminal/palettes";

export function hostNeedsAttention(detail: HostDetail): boolean {
  return (detail.form.notices ?? []).length > 0 || (detail.effective.notices ?? []).length > 0;
}

function inherited(detail: HostDetail) {
  const own = detail.form.entry.file.path ?? detail.form.entry.file.absolute;
  return detail.effective.entries.filter((entry) => (entry.source.path ?? entry.source.absolute) !== own);
}

export function HostInspector({
  detail,
  onMetadata,
}: {
  detail: HostDetail;
  onMetadata: (metadata: HostMetadata) => void;
}) {
  const t = useTranslate();
  const notices = [...(detail.form.notices ?? []), ...(detail.effective.notices ?? [])];
  const fromElsewhere = inherited(detail);

  return (
    <div className="flex flex-col gap-6">

      <section className="flex flex-col gap-3">
        <div>
          <h3 className="text-sm font-semibold text-ink">{t("inspector.appOnly")}</h3>
          <p className={`mt-1 ${hintText}`}>{t("inspector.hostSavesImmediately")}</p>
        </div>

        <div className="flex flex-col gap-4 rounded-md bg-tree p-4">
        <div className="flex flex-col gap-2">
          <Field label={t("host.colour")}>
            <input
              type="color"
              value={
                detail.metadata.colour === undefined || detail.metadata.colour === ""
                  ? "#8e8e93" /* palette-exempt: ネイティブコントロール自身の中立色 */
                  : detail.metadata.colour
              }
              onChange={(event) => onMetadata({ ...detail.metadata, colour: event.target.value })}
              className="h-8 w-14 rounded border border-control-line bg-control"
            />
          </Field>
          {detail.metadata.colour === undefined || detail.metadata.colour === "" ? null : (
            <Button className="self-start" onClick={() => onMetadata({ ...detail.metadata, colour: "" })}>
              {t("host.clearColour")}
            </Button>
          )}
        </div>

        <Field label={t("connection.paletteLabel")} hint={t("connection.paletteHint")}>
          <AppearancePicker
            choices={palettes}
            value={detail.metadata.appearance?.palette ?? ""}
            onChange={(chosen) => onMetadata(chooseAppearance(detail.metadata, { palette: chosen }))}
            unchosen={t("terminal.paletteFollowsOverall")}
          />
        </Field>

        <Field label={t("connection.fontLabel")} hint={t("connection.fontHint")}>
          <AppearancePicker
            choices={fonts}
            value={detail.metadata.appearance?.font ?? ""}
            onChange={(chosen) => onMetadata(chooseAppearance(detail.metadata, { font: chosen }))}
            unchosen={t("terminal.fontFollowsOverall")}
          />
        </Field>

        <Field label={t("connection.backgroundLabel")} hint={t("connection.backgroundHint")}>
          <BackgroundPicker
            value={detail.metadata.appearance?.background ?? ""}
            onChange={(chosen) => onMetadata(chooseAppearance(detail.metadata, { background: chosen }))}
            tint={detail.metadata.appearance?.backgroundTint}
            onTintChange={(chosen) => onMetadata(chooseAppearance(detail.metadata, { backgroundTint: chosen }))}
            unchosen={t("terminal.backgroundFollowsOverall")}
          />
        </Field>

        <Field label={t("connection.encodingLabel")} hint={t("connection.encodingHint")}>
          <select
            value={detail.metadata.encoding ?? ""}
            onChange={(event) => {
              const metadata = { ...detail.metadata };
              const encoding = event.target.value as NonNullable<HostMetadata["encoding"]> | "";
              if (encoding === "") {
                delete metadata.encoding;
              } else {
                metadata.encoding = encoding;
              }
              onMetadata(metadata);
            }}
            className={control}
          >
            <option value="">{t("connection.encodingUTF8")}</option>
            <option value="shift_jis">{t("connection.encodingShiftJIS")}</option>
            <option value="euc-jp">{t("connection.encodingEUCJP")}</option>
            <option value="iso-2022-jp">{t("connection.encodingISO2022JP")}</option>
          </select>
        </Field>

        <Field label={t("host.tags")}>
          <input
            value={(detail.metadata.tags ?? []).join(", ")}
            onChange={(event) =>
              onMetadata({
                ...detail.metadata,
                tags: event.target.value
                  .split(",")
                  .map((tag) => tag.trim())
                  .filter((tag) => tag !== ""),
              })
            }
            className={control}
          />
        </Field>

        <Field label={t("host.displayOrder")}>
          <input
            type="number"
            value={String(detail.metadata.order ?? 0)}
            onChange={(event) => onMetadata({ ...detail.metadata, order: Number(event.target.value) || 0 })}
            className={control}
          />
        </Field>
        </div>
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
