import { useTranslate } from "../i18n/context";
import { Segmented } from "../ui/surface";
import type { SFTPEntryView } from "./sftpPanes";

// The choice between the rows and the grid, above a list on any screen.
export function SFTPEntryViewSwitch({ view, onChange, touchCompact }: {
  view: SFTPEntryView;
  onChange: (view: SFTPEntryView) => void;
  touchCompact: boolean;
}) {
  const t = useTranslate();
  return (
    <Segmented
      label={t("sftp.entryView")}
      value={view}
      options={[
        { value: "list", label: t("sftp.entryViewList") },
        { value: "grid", label: t("sftp.entryViewGrid") },
      ]}
      onChange={onChange}
      touchCompact={touchCompact}
    />
  );
}
