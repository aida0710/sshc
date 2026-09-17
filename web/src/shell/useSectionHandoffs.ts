import { useCallback, useRef, useState } from "react";
import type { CreateConnectionDraft } from "../connections/CreateConnectionModal";
import type { FileTarget } from "../explorer/ConfigExplorer";
import type { GeneratedPrivateKeyHandoff, GeneratedPublicKeyHandoff } from "../keys/workflow";
import type { Section } from "../routing/sectionRoute";
import type { SFTPTarget } from "../sftp/SFTPPanel";
import type { RemotePathAction } from "../terminal/TerminalLinkPopover";

// Things one section hands to another as the user is sent there: a config
// line to open, a remote path for the file browser, a key just generated for
// a connection form or a server, and a connection draft parked while its
// prerequisite is created. Each is consumed by the section that receives it.
export function useSectionHandoffs(navigate: (section: Section) => void) {
  const [fileTarget, setFileTarget] = useState<FileTarget | null>(null);
  const [sftpTarget, setSftpTarget] = useState<SFTPTarget | null>(null);
  const sftpTargetSequence = useRef(0);
  const [connectionDraft, setConnectionDraft] = useState<CreateConnectionDraft | null>(null);
  const [connectionKey, setConnectionKey] = useState<GeneratedPrivateKeyHandoff | null>(null);
  const [publicKey, setPublicKey] = useState<GeneratedPublicKeyHandoff | null>(null);
  const consumeConnectionKey = useCallback(() => setConnectionKey(null), []);
  const consumePublicKey = useCallback(() => setPublicKey(null), []);

  function openFile(path: string, line: number) {
    setFileTarget({ path, line });
    navigate("Config");
  }

  function openRemotePath(alias: string, path: string, action: RemotePathAction) {
    sftpTargetSequence.current += 1;
    setSftpTarget({ alias, path, action, request: sftpTargetSequence.current });
    navigate("Files");
  }

  function handleSftpTarget(request: number) {
    setSftpTarget((current) => current?.request === request ? null : current);
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
    fileTarget,
    sftpTarget,
    connectionDraft,
    setConnectionDraft,
    connectionKey,
    publicKey,
    consumeConnectionKey,
    consumePublicKey,
    openFile,
    openRemotePath,
    handleSftpTarget,
    assignGeneratedKey,
    installGeneratedKey,
  };
}
