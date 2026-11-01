import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { PreviewIconButton, previewIconButtonClassName } from "./PreviewIconButton";
import { PreviewStepper } from "./PreviewStepper";

// FilePreviewHeader names the shown file and holds the preview's controls:
// stepping through the other printed files, downloading, the "…" of what
// else the file offers (menu), and closing. On a narrow screen the path takes
// the first row on its own and the controls move to the second.
//
// overlay draws the header for a place over a full-screen picture, as a phone's
// photos do: no bar of its own, and light words on the shade the caller lays.
export function FilePreviewHeader({
  headingId,
  path,
  position,
  count,
  onStep,
  onDownload,
  menu,
  overlay = false,
  onClose,
}: {
  headingId: string;
  path: string;
  position: number;
  count: number;
  onStep: (position: number) => void;
  onDownload: () => void;
  menu?: { label: string; items: ActionMenuItem[] };
  overlay?: boolean;
  onClose: () => void;
}) {
  const t = useTranslate();
  return (
    <div className={`flex flex-wrap items-center gap-x-2 gap-y-1 px-3 py-2 ${overlay
      ? "text-white [&_button]:text-white/85"
      : "border-b border-line bg-toolbar"}`}>
      {/* A long path loses its start, not the file name: the right-to-left box
          puts the ellipsis on the left, and the bdi keeps the path itself in
          reading order. */}
      <h2 id={headingId} dir="rtl" className="min-w-0 basis-full truncate text-left font-mono text-sm font-medium md:basis-0 md:grow" title={path}>
        <bdi dir="ltr">{path}</bdi>
      </h2>
      <div className="ml-auto flex shrink-0 items-center gap-1">
        <PreviewStepper position={position} count={count} onStep={onStep} />
        <PreviewIconButton icon="download" label={t("sftp.download")} onClick={onDownload} />
        {menu === undefined ? null : (
          <ActionMenu label={menu.label} items={menu.items} triggerClassName={previewIconButtonClassName} />
        )}
        <PreviewIconButton icon="close" label={t("sftp.close")} onClick={onClose} />
      </div>
    </div>
  );
}
