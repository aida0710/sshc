import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { PathText } from "../ui/PathText";
import { PreviewIconButton, previewIconButtonClassName } from "./PreviewIconButton";
import { PreviewStepper } from "./PreviewStepper";

function folderOf(path: string): string {
  return path.slice(0, path.lastIndexOf("/")) || "/";
}

// FilePreviewHeader names the shown file and holds the preview's controls:
// stepping through the other files, downloading, the "…" of what else the file
// offers (menu), and closing. The name comes first and is never cut short by the
// controls: on a narrow screen it takes the first row on its own, and the folder
// and the controls share the second.
//
// path is where the shown entry is; several entries shown together have none.
// accessibleName is what a screen reader calls the window.
//
// overlay draws the header for a place over a full-screen picture, as a phone's
// photos do: no bar of its own, and light words on the shade the caller lays.
export function FilePreviewHeader({
  headingId,
  heading,
  path,
  accessibleName,
  position,
  count,
  onStep,
  onDownload,
  downloadDisabled = false,
  menu,
  overlay = false,
  onClose,
}: {
  headingId: string;
  heading: string;
  path?: string;
  accessibleName?: string;
  position: number;
  count: number;
  onStep: (position: number) => void;
  onDownload?: (() => void) | undefined;
  downloadDisabled?: boolean;
  menu?: { label: string; items: ActionMenuItem[] } | undefined;
  overlay?: boolean;
  onClose: () => void;
}) {
  const t = useTranslate();
  return (
    <div className={`flex flex-wrap items-center gap-x-2 gap-y-1 px-3 py-2 ${overlay
      ? "text-white [&_button]:text-white/85"
      : "border-b border-line bg-toolbar"}`}>
      <h2 id={headingId} aria-label={accessibleName} title={path} className="line-clamp-2 min-w-0 basis-full break-all font-mono text-sm font-medium md:basis-0 md:grow">
        {heading}
      </h2>
      {path === undefined ? null : (
        <PathText path={folderOf(path)} className={`flex-1 font-mono text-xs md:max-w-[40%] md:flex-none ${overlay ? "text-white/70" : "text-ink-muted"}`} />
      )}
      <div className="ml-auto flex shrink-0 items-center gap-1">
        <PreviewStepper position={position} count={count} onStep={onStep} />
        {onDownload === undefined ? null : <PreviewIconButton icon="download" label={t("sftp.download")} disabled={downloadDisabled} onClick={onDownload} />}
        {menu === undefined ? null : (
          <ActionMenu label={menu.label} items={menu.items} triggerClassName={previewIconButtonClassName} />
        )}
        <PreviewIconButton icon="close" label={t("sftp.close")} onClick={onClose} />
      </div>
    </div>
  );
}
