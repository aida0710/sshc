import { useId, useState } from "react";
import { useTranslate } from "../i18n/context";
import type { ActionMenuItem } from "../ui/ActionMenu";
import { ModalShell } from "../ui/ModalShell";
import { FilePreviewBody } from "./FilePreviewBody";
import { FilePreviewHeader } from "./FilePreviewHeader";
import { useFilePreview } from "./useFilePreview";
import { usePreviewDownload } from "./usePreviewDownload";
import { usePreviewStepping } from "./usePreviewStepping";

function baseName(path: string): string {
  const components = path.split("/").filter(Boolean);
  return components[components.length - 1] ?? path;
}

// FilePreviewDialog shows one file over the screen that named it, such as a
// path a terminal printed. The file is read when it is shown, never before,
// so a program printing many paths does not make sshc read them. Every file
// can be downloaded from here, including those it cannot show.
//
// paths are the files the arrow keys step through, in order. When path is
// not among them, the dialog shows path alone. actions are what else the
// shown file offers, in the header's "…".
export function FilePreviewDialog({
  alias,
  path,
  paths = [],
  actions,
  onClose,
}: {
  alias: string;
  path: string;
  paths?: string[];
  actions?: (path: string) => ActionMenuItem[];
  onClose: () => void;
}) {
  const t = useTranslate();
  const headingId = useId();
  const steps = paths.includes(path) ? paths : [path];
  const [position, setPosition] = useState(() => steps.indexOf(path));
  const shown = steps[position] ?? path;
  const preview = useFilePreview(alias, shown);
  const { download, problem } = usePreviewDownload(alias);
  const onSwipe = usePreviewStepping({ position, count: steps.length, onStep: setPosition, onClose });

  return (
    <ModalShell
      labelledBy={headingId}
      onDismiss={onClose}
      closeOnOutside
      panelClassName="flex h-[min(46rem,calc(100dvh-2rem))] w-full max-w-4xl flex-col overflow-hidden rounded-lg"
    >
      <FilePreviewHeader
        headingId={headingId}
        path={shown}
        position={position}
        count={steps.length}
        onStep={setPosition}
        onDownload={() => download(shown)}
        {...(actions === undefined ? {} : { menu: { label: t("sftp.selectedActions", { name: baseName(shown) }), items: actions(shown) } })}
        onClose={onClose}
      />
      {problem === "" ? null : <p role="alert" className="border-b border-line px-3 py-2 text-xs text-danger">{problem}</p>}
      <div role="group" className="min-h-0 flex-1 overflow-auto overscroll-contain bg-surface-subtle p-3" aria-label={t("sftp.preview")}>
        <FilePreviewBody preview={preview} path={shown} name={baseName(shown)} onDownload={() => download(shown)} onSwipe={onSwipe} />
      </div>
    </ModalShell>
  );
}
