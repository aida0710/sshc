import { useCallback, useRef, useState } from "react";
import type { CreateConnectionDraft } from "../connections/CreateConnectionModal";
import type { FileTarget } from "../explorer/ConfigExplorer";
import type { GeneratedPrivateKeyHandoff, GeneratedPublicKeyHandoff } from "../keys/workflow";
import type { Section } from "../routing/sectionRoute";
import type { SFTPTarget } from "../sftp/SFTPPanel";
import type { RemotePathAction } from "../terminal/TerminalLinkPopover";

// A handoff the receiving section clears by the request number it took, so a
// newer handoff sent while the section was acting on the old one survives.
function useNumberedHandoff<Target>() {
  const [target, setTarget] = useState<(Target & { request: number }) | null>(null);
  const sequence = useRef(0);
  const send = useCallback((next: Target) => {
    sequence.current += 1;
    setTarget({ ...next, request: sequence.current });
  }, []);
  const consume = useCallback((request: number) => {
    setTarget((current) => current?.request === request ? null : current);
  }, []);
  return { target, send, consume };
}

// Things one section hands to another as the user is sent there: a config
// line to open, a remote path for the file browser, a key just generated for
// a connection form or a server, and a connection draft parked while its
// prerequisite is created. Each is consumed by the section that receives it,
// so coming back to that section later starts from its own default view.
export function useSectionHandoffs(navigate: (section: Section) => void) {
  const fileHandoff = useNumberedHandoff<Omit<FileTarget, "request">>();
  const sftpHandoff = useNumberedHandoff<Omit<SFTPTarget, "request">>();
  const [connectionDraft, setConnectionDraft] = useState<CreateConnectionDraft | null>(null);
  const [connectionKey, setConnectionKey] = useState<GeneratedPrivateKeyHandoff | null>(null);
  const [publicKey, setPublicKey] = useState<GeneratedPublicKeyHandoff | null>(null);
  const consumeConnectionKey = useCallback(() => setConnectionKey(null), []);
  const consumePublicKey = useCallback(() => setPublicKey(null), []);

  function openFile(path: string, line: number) {
    fileHandoff.send({ path, line });
    navigate("Config");
  }

  function openRemotePath(alias: string, path: string, action: RemotePathAction) {
    sftpHandoff.send({ alias, path, action });
    navigate("Files");
  }

  function assignGeneratedKey(key: GeneratedPrivateKeyHandoff) {
    setConnectionKey(key);
    navigate("Connections");
  }

  function installGeneratedKey(key: GeneratedPublicKeyHandoff) {
    setPublicKey(key);
    navigate("Remote Keys");
  }

  return {
    fileTarget: fileHandoff.target,
    consumeFileTarget: fileHandoff.consume,
    sftpTarget: sftpHandoff.target,
    consumeSftpTarget: sftpHandoff.consume,
    connectionDraft,
    setConnectionDraft,
    connectionKey,
    publicKey,
    consumeConnectionKey,
    consumePublicKey,
    openFile,
    openRemotePath,
    assignGeneratedKey,
    installGeneratedKey,
  };
}
