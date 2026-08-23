import type { GroupMetadata } from "../api/config";
import { CheckboxField, Field, control, fieldLabel, hintText } from "../ui/form";
import { Button, Card } from "../ui/surface";
import { useTranslate } from "../i18n/context";

export function GroupInspector({
  group,
  members,
  onUpdate,
}: {
  group: GroupMetadata;
  members: string[];
  onUpdate: (patch: Partial<GroupMetadata>) => void;
}) {
  const t = useTranslate();
  const colour = group.colour === undefined || group.colour === "" ? "" : group.colour;

  return (
    <div className="flex flex-col gap-4">
      <h3 className={fieldLabel}>{t("inspector.appOnly")}</h3>
      <p className={hintText}>{t("inspector.groupChangesStaged")}</p>

      <Card padded>
        <Field label={t("groups.colour")}>
          <input
            id={`group-colour-${group.name}`}
            type="color"
            value={colour === "" ? "#8e8e93" /* palette-exempt: ネイティブコントロール自身の中立色 */ : colour}
            onChange={(event) => onUpdate({ colour: event.target.value })}
            className="h-8 w-14 rounded border border-control-line bg-control"
          />
        </Field>
        {colour === "" ? null : (
          <Button className="self-start" onClick={() => onUpdate({ colour: "" })}>
            {t("groups.clearColour", { name: group.name })}
          </Button>
        )}

        <Field label={t("groups.displayOrder")}>
          <input
            id={`group-order-${group.name}`}
            type="number"
            value={String(group.order ?? 0)}
            onChange={(event) => onUpdate({ order: Number(event.target.value) || 0 })}
            className={control}
          />
        </Field>


        <CheckboxField
          label={t("groups.hide", { name: group.name })}
          checked={group.hidden === true}
          disabled={members.length > 0}
          onChange={(checked) => onUpdate({ hidden: checked })}
        />
        {members.length === 0 ? null : <p className={hintText}>{t("groups.hideOnlyContainers")}</p>}
      </Card>
    </div>
  );
}
