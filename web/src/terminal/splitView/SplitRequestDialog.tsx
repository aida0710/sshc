import { useEffect, useState } from "react";
import type { HostEntry } from "../../api/config";
import type { LocalShellProfile } from "../../api/settings";
import type { OpenTerminalSessionRequest, TerminalSession } from "../../api/terminalSessions";
import { sshSessionTarget } from "../../connections/directDestination";
import { useTranslate } from "../../i18n/context";
import { HostPickerDialog, type LocalChoice } from "../../shell/HostPickerDialog";
import type { DirectDestinationActions } from "../../shell/useDirectDestinationActions";
import { duplicateSessionRequest } from "../duplicateSessionRequest";
import { localShellChoices, shellRequest } from "../localShellChoices";
import { terminalDisplayTitle } from "../terminalPresentation";
import { SplitSessionPicker } from "./SplitSessionPicker";
import type { SplitRequest, SplitViewController } from "./useSplitView";

// A profile's id comes from the engine, so the duplicate's id is one no profile has.
const duplicateChoiceId = "duplicate:session";

type SplitRequestDialogProps = {
  splitView: SplitViewController;
  sessions: TerminalSession[];
  aliases: string[];
  hosts: HostEntry[];
  localShellProfiles: LocalShellProfile[];
  // runTarget is the SSH alias of the engine the shells open on, or null for this machine.
  runTarget: string | null;
  // directDestination names what a user@host[:port] typed into the search would be saved as.
  directDestination: Pick<DirectDestinationActions, "aliasFor">;
  onOpen: (request: OpenTerminalSessionRequest, place: SplitRequest) => void;
};

// SplitRequestDialog asks what goes beside a session when 右に分割 or 下に分割 is chosen. It lists the sessions that
// can go there and offers a new one; with none to list, it goes straight to the choice of a new session.
export function SplitRequestDialog(props: SplitRequestDialogProps) {
  const request = props.splitView.splitRequest;
  return request === null ? null : <SplitRequestChoice {...props} request={request} />;
}

function SplitRequestChoice({ splitView, sessions, aliases, hosts, localShellProfiles, runTarget, directDestination, onOpen, request }: SplitRequestDialogProps & { request: SplitRequest }) {
  const t = useTranslate();
  const [choosingNew, setChoosingNew] = useState(false);
  const { targetSessionId } = request;
  const { cancelSplitRequest, selectSession } = splitView;
  const target = sessions.find((session) => session.id === targetSessionId);
  const candidates = sessions.filter((candidate) => splitView.canSplit(candidate.id, targetSessionId));

  useEffect(() => {
    if (target === undefined) cancelSplitRequest();
  }, [target, cancelSplitRequest]);
  if (target === undefined) return null;

  // The menu that asked is gone, so the keyboard goes back to the terminal it was opened on.
  const cancel = () => {
    cancelSplitRequest();
    selectSession(targetSessionId);
  };
  const open = (opening: OpenTerminalSessionRequest) => {
    cancelSplitRequest();
    onOpen(opening, request);
  };

  if (candidates.length > 0 && !choosingNew) {
    return (
      <SplitSessionPicker
        sessions={candidates}
        {...(splitView.canOpenBeside(targetSessionId) ? { onOpenNew: () => setChoosingNew(true) } : {})}
        onClose={cancel}
        onSelect={(sourceSessionId) => {
          splitView.split({ ...request, sourceSessionId });
          cancelSplitRequest();
        }}
      />
    );
  }
  const duplicate: LocalChoice = { id: duplicateChoiceId, label: t("terminal.duplicate"), detail: terminalDisplayTitle(target) };
  return (
    <HostPickerDialog
      open
      heading={t(request.edge === "right" ? "splitView.newSessionRight" : "splitView.newSessionBelow")}
      aliases={aliases}
      hosts={hosts}
      local={[duplicate, ...localShellChoices(t, localShellProfiles, runTarget)]}
      // Connecting to a typed destination is offered; saving it first is not, which would open the session outside the split.
      direct={{ aliasFor: directDestination.aliasFor, connect: (destination) => open({ kind: "ssh", ...sshSessionTarget(destination.text), workspaceId: target.workspaceId }) }}
      onChoose={(alias) => open({ kind: "ssh", ...sshSessionTarget(alias), workspaceId: target.workspaceId })}
      onChooseLocal={(id) => open(id === duplicateChoiceId ? duplicateSessionRequest(target) : shellRequest(id, target.workspaceId))}
      onClose={cancel}
    />
  );
}
