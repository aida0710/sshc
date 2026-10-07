import { useRef, useState } from "react";
import { ApiError } from "../api/client";
import { useTranslate } from "../i18n/context";
import { sftpApi, type RemoteEntry } from "./api";
import { chmodApi, chmodRefusalCodes, type ChmodOptions, type ChmodSelection } from "./chmodApi";
import { remoteParentOf } from "./sftpSource";
import { sftpProblemText } from "./sftpProblemText";
import { symbolicModeToOctal } from "./transfers";
import type { SFTPBrowserModel } from "./useSFTPBrowser";
import type { ConfirmedPermissions, PermissionsIntent } from "./permissionsIntent";

// The host, selection and options are captured before planning. Navigating or
// cancelling during a read cannot move its response into another confirmation.
export function useSFTPPermissions({ browser, refreshAfterChange, offerUndo, onInteract }: {
  browser: SFTPBrowserModel;
  refreshAfterChange: (directory: string, alias: string) => Promise<unknown>;
  offerUndo: (label: string, run: () => Promise<void>) => void;
  onInteract: () => void;
}) {
  const t = useTranslate();
  const [intent, setIntent] = useState<PermissionsIntent | null>(null);
  const opened = useRef<PermissionsIntent | null>(null);
  const [confirmation, setConfirmation] = useState<ConfirmedPermissions | null>(null);
  const [planning, setPlanning] = useState(false);
  const [applying, setApplying] = useState(false);
  const [attempted, setAttempted] = useState(false);
  const [problem, setProblem] = useState("");
  const running = useRef(false);
  const planningIntent = useRef<PermissionsIntent | null>(null);

  function open(next: PermissionsIntent) {
    opened.current = next;
    planningIntent.current = null;
    setIntent(next);
    setConfirmation(null);
    setProblem("");
    setPlanning(false);
    setAttempted(false);
    onInteract();
  }

  async function review(options: ChmodOptions) {
    const selected = opened.current;
    if (selected === null || planningIntent.current === selected || running.current) return;
    planningIntent.current = selected;
    const selection: ChmodSelection = {
      entries: selected.entries.map((entry) => ({ path: entry.path, expectedRevision: entry.revision })), options: { ...options },
    };
    setPlanning(true);
    setProblem("");
    try {
      const plan = await chmodApi.plan(selected.alias, selection);
      if (opened.current !== selected) return;
      setConfirmation({ selection, plan });
    } catch (error) {
      if (opened.current === selected) setProblem(error instanceof ApiError && error.code === "sftp_unsupported_operation" ? t("sftp.chmodUnsupported") : sftpProblemText(t, error));
    } finally {
      if (planningIntent.current === selected) planningIntent.current = null;
      if (opened.current === selected) setPlanning(false);
    }
  }

  function cancel() {
    if (running.current) return;
    opened.current = null;
    setIntent(null);
    setConfirmation(null);
    setPlanning(false);
  }

  function offerPermissionUndo(selected: PermissionsIntent, options: ChmodOptions) {
    const entry = selected.entries[0];
    if (selected.entries.length !== 1 || options.recursive || entry === undefined) return;
    const previous = symbolicModeToOctal(entry.mode);
    const mode = entry.type === "directory" ? options.directoryMode : options.fileMode;
    if (previous === mode) return;
    offerUndo(t("sftp.permissionsChanged", { mode }), async () => {
      const isCurrent = browser.generation.observe();
      const previousIntent = opened.current;
      const listing = await sftpApi.list(selected.alias, remoteParentOf(entry.path));
      if (!isCurrent() || opened.current !== previousIntent || running.current) return;
      const current = listing.entries.find((candidate) => candidate.path === entry.path);
      if (current === undefined) throw new ApiError("sftp_conflict", 409, null);
      open({ ...selected, entries: [current], recursive: false, initialMode: previous, isCurrent });
    });
  }

  async function refreshAfterApply(selected: PermissionsIntent, options: ChmodOptions, complete: boolean) {
    try {
      await refreshAfterChange(selected.directory, selected.alias);
      if (complete) offerPermissionUndo(selected, options);
    } catch (error) {
      browser.setProblem(sftpProblemText(t, error));
    }
  }

  async function apply() {
    const selected = opened.current;
    if (selected === null || confirmation === null || attempted || running.current) return;
    running.current = true;
    setApplying(true);
    setAttempted(true);
    setProblem("");
    try {
      const result = await chmodApi.apply({ alias: selected.alias, ...confirmation });
      if (!result.complete) {
        setProblem(t("sftp.chmodPartial", { applied: result.applied, count: result.items }));
      } else {
        opened.current = null;
        setIntent(null);
        setConfirmation(null);
      }
      if (selected.isCurrent()) {
        await refreshAfterApply(selected, confirmation.selection.options, result.complete);
      }
    } catch (error) {
      // A lost response may follow successful SETSTAT requests. A consumed
      // confirmation must never be retried automatically from this dialog.
      const refusedBeforeChange = error instanceof ApiError && chmodRefusalCodes.includes(error.code);
      setProblem(`${sftpProblemText(t, error)} ${t(refusedBeforeChange ? "sftp.chmodCheckAfterRefusal" : "sftp.chmodCheckAfterFailure")}`);
    } finally {
      running.current = false;
      setApplying(false);
    }
  }

  return {
    intent, confirmation, planning, applying, attempted, problem, review, apply, cancel,
    ask(entries: RemoteEntry[], recursive = false) {
      if (running.current || !browser.source?.can.chmod || entries.length === 0 || entries.some((entry) => entry.type !== "file" && entry.type !== "directory")) return;
      open({ entries: entries.map((entry) => ({ ...entry })), alias: browser.alias, directory: browser.path,
        recursive, isCurrent: browser.generation.observe() });
    },
  };
}

export type SFTPPermissionsModel = ReturnType<typeof useSFTPPermissions>;
