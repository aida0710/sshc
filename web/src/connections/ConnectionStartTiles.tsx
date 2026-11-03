import { useTranslate } from "../i18n/context";
import { StartTiles } from "../ui/StartTiles";

// ConnectionStartTiles is what Home and Connections show while there is no connection: the ways to get the
// first one, and a local shell for someone who needs none.
export function ConnectionStartTiles({
  onAddConnection,
  onOpenConfig,
  onOpenSync,
  onOpenLocalShell,
}: {
  onAddConnection: () => void;
  onOpenConfig: () => void;
  onOpenSync: () => void;
  onOpenLocalShell: () => void;
}) {
  const t = useTranslate();
  return (
    <StartTiles
      heading={t("start.heading")}
      brand
      tiles={[
        { icon: "connections", title: t("conn.new"), note: t("start.addConnectionNote"), primary: true, onSelect: onAddConnection },
        { icon: "config", title: t("start.openConfig"), note: t("start.openConfigNote"), onSelect: onOpenConfig },
        { icon: "sync", title: t("start.receiveSync"), note: t("start.receiveSyncNote"), onSelect: onOpenSync },
      ]}
      alternative={{ lead: t("start.withoutConnection"), label: t("start.openLocalShell"), icon: "terminal", onSelect: onOpenLocalShell }}
    />
  );
}
