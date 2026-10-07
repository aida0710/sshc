import { useState } from "react";
import { useTranslate } from "../i18n/context";
import type { RemoteEntry } from "./api";
import type { SFTPBrowserModel } from "./useSFTPBrowser";
import { remoteMetadataApi } from "./remoteMetadataApi";
import { remoteJoin } from "./sftpSource";
import { sftpProblemText } from "./sftpProblemText";

export type MetadataIntent =
  | { kind: "createSymlink" }
  | { kind: "changeSymlink"; entry: RemoteEntry }
  | { kind: "ownership"; entry: RemoteEntry };

export type MetadataValues = { name: string; target: string; uid: number; gid: number };

// A dialog holds the host and directory where it opened until its operation completes.
export function useSFTPMetadataActions(browser: SFTPBrowserModel, refreshAfterChange: (path: string, alias: string) => Promise<unknown>) {
  const t = useTranslate();
  const [opened, setOpened] = useState<{
    intent: MetadataIntent; alias: string; directory: string; isCurrent: () => boolean;
  } | null>(null);
  const [acting, setActing] = useState(false);
  const [problem, setProblem] = useState("");

  async function submit(values: MetadataValues) {
    if (opened === null || acting) return;
    const { intent, alias, directory, isCurrent } = opened;
    setActing(true);
    setProblem("");
    try {
      if (intent.kind === "createSymlink") {
        await remoteMetadataApi.createSymlink(alias, { path: remoteJoin(directory, values.name), target: values.target });
      } else if (intent.kind === "changeSymlink") {
        await remoteMetadataApi.changeSymlink(alias, { path: intent.entry.path, target: values.target, expectedRevision: intent.entry.revision });
      } else {
        await remoteMetadataApi.changeOwnership(alias, { path: intent.entry.path, uid: values.uid, gid: values.gid, expectedRevision: intent.entry.revision });
      }
      setOpened(null);
      if (isCurrent()) await refreshAfterChange(directory, alias);
    } catch (error) {
      setProblem(sftpProblemText(t, error));
    } finally {
      setActing(false);
    }
  }

  return {
    intent: opened?.intent ?? null,
    directory: opened?.directory ?? "",
    acting, problem, submit,
    ask(intent: MetadataIntent) {
      setProblem("");
      setOpened({ intent, alias: browser.alias, directory: browser.path, isCurrent: browser.generation.observe() });
    },
    cancel() { if (!acting) setOpened(null); },
  };
}

export type SFTPMetadataActionsModel = ReturnType<typeof useSFTPMetadataActions>;
