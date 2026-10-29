import { useRef, useState, type RefObject } from "react";
import type { Terminal } from "@xterm/xterm";
import type { TerminalSession, TerminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import type { RemotePathAction } from "../sftp/SFTPPanel";
import type { ActionMenuItem } from "../ui/ActionMenu";
import { clipboard } from "../ui/clipboard";
import { useToast } from "../ui/useToast";
import { pathPreviewAlias } from "./pathPreviewAlias";
import { pathPreviewHome } from "./pathPreviewHome";
import { previewPathsInBuffer, previewPathTargetsInBuffer } from "./previewPaths";
import { isHomePath, resolveTerminalPath } from "./relativePaths";
import { useSameDestination } from "./useSameDestination";

export type TerminalPathPreview = {
  // previewAlias is the connection whose files the preview reads; undefined where this session's files
  // cannot be read here.
  previewAlias: string | undefined;
  // shown is the file being previewed, with the other paths on screen to step through.
  shown: { path: string; paths: string[] } | null;
  // previewLinkedPath opens a clicked file path's preview at once; null where this session's files cannot
  // be read. It is a ref so the terminal, built once, reads the current one.
  previewLinkedPath: RefObject<((path: string) => void) | null>;
  // fileActions are what the preview's "…" offers for a file.
  fileActions: (path: string) => ActionMenuItem[];
  close: () => void;
};

// useTerminalPathPreview previews a file path printed in the terminal, reading it from the session's host.
export function useTerminalPathPreview({ session, runTarget, api, terminal, currentDirectory, allowWindowsPaths, onOpenRemotePath, notice }: {
  session: TerminalSession;
  runTarget: string | undefined;
  api: Pick<TerminalSessionsApi, "terminalSessionDirectory">;
  terminal: RefObject<Terminal | null>;
  // currentDirectory is the directory the shell last reported through OSC 7 or OSC 9;9.
  currentDirectory: string;
  allowWindowsPaths: boolean;
  onOpenRemotePath: ((alias: string, path: string, action: RemotePathAction) => void) | undefined;
  notice: (message: string) => void;
}): TerminalPathPreview {
  const t = useTranslate();
  const showToast = useToast();
  const [shown, setShown] = useState<{ path: string; paths: string[] } | null>(null);
  const previewLinkedPath = useRef<((path: string) => void) | null>(null);
  const remoteAlias = session.kind === "ssh" ? session.alias : undefined;
  const sameDestination = useSameDestination(remoteAlias, runTarget);
  const previewAlias = pathPreviewAlias(session, runTarget, sameDestination);

  // A relative path is relative to where the program that printed it works.
  // The engine reads that for a local shell; an SSH session only has the
  // directory its shell reported through OSC 7.
  async function previewPath(path: string) {
    if (previewAlias === undefined) return;
    const buffer = terminal.current?.buffer.active;
    const needsHome = isHomePath(path) || (buffer !== undefined && previewPathTargetsInBuffer(buffer, allowWindowsPaths).some(isHomePath));
    const [engineDirectory, home] = await Promise.all([
      api.terminalSessionDirectory(session.id).catch(() => null),
      needsHome ? pathPreviewHome(previewAlias).catch(() => null) : null,
    ]);
    const directory = engineDirectory ?? (currentDirectory === "" ? null : currentDirectory);
    const resolvedPath = resolveTerminalPath(path, directory, home);
    if (resolvedPath === null) {
      notice(t(isHomePath(path) ? "terminal.homePathUnknown" : "terminal.relativePathUnknown"));
      return;
    }
    setShown({
      path: resolvedPath,
      paths: terminal.current === null ? [] : previewPathsInBuffer(terminal.current.buffer.active, { directory, home, allowWindowsPaths }),
    });
  }
  previewLinkedPath.current = previewAlias === undefined ? null : (path) => void previewPath(path);

  // SFTP can open a file of an SSH session at its host. A session opened on a run target reached its host
  // through that engine's SSH config, which this machine may not share; only the same destination will do.
  const openInSFTP = (runTarget === undefined || sameDestination) && remoteAlias !== undefined && onOpenRemotePath !== undefined
    ? (path: string, action: RemotePathAction) => {
      setShown(null);
      onOpenRemotePath(remoteAlias, path, action);
    }
    : undefined;
  function fileActions(path: string): ActionMenuItem[] {
    return [
      ...(openInSFTP === undefined ? [] : [
        { label: t("terminal.linkBrowseSFTP"), onSelect: () => openInSFTP(path, "browse") },
        { label: t("terminal.linkEditSFTP"), onSelect: () => openInSFTP(path, "edit") },
        { label: t("terminal.linkDownloadSFTP"), onSelect: () => openInSFTP(path, "download") },
      ]),
      {
        label: t("sftp.copyPath"),
        onSelect: () => void clipboard.writeText(path)
          .then(() => showToast(t("copy.done"), { tone: "success" }))
          .catch(() => showToast(t("copy.refused"))),
      },
    ];
  }

  return { previewAlias, shown, previewLinkedPath, fileActions, close: () => setShown(null) };
}
