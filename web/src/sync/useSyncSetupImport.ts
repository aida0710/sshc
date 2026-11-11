import { useState } from "react";
import type { SyncDirection, SyncSetupImportRequest } from "../api/sync";
import type { MessageKey } from "../i18n/messages";
import { looksLikeSyncSetupExport } from "./syncSetupExportDocument";

type SetupImportState = {
  // The exported document, once a file or a QR code has given one. Empty until then.
  document: string;
  key: string;
  direction: SyncDirection;
  scanning: boolean;
  // Why the chosen file was not taken.
  fileProblem: MessageKey | null;
};

const initialState: SetupImportState = { document: "", key: "", direction: "both", scanning: false, fileProblem: null };

// What the person has given so far to set this machine up from another machine's exported settings.
export function useSyncSetupImport() {
  const [state, setState] = useState<SetupImportState>(initialState);

  async function readFile(file: File) {
    let text: string;
    try {
      text = await file.text();
    } catch {
      setState((current) => ({ ...current, fileProblem: "sync.import.fileReadFailed" }));
      return;
    }
    setState((current) =>
      looksLikeSyncSetupExport(text)
        ? { ...current, document: text, fileProblem: null }
        : { ...current, fileProblem: "sync.import.notExport" },
    );
  }

  // Takes the text of a QR code the camera saw. Any other code in view is passed over, so the camera keeps
  // looking; true ends the scan.
  function acceptScannedText(text: string): boolean {
    if (!looksLikeSyncSetupExport(text)) return false;
    setState((current) => ({ ...current, document: text, scanning: false, fileProblem: null }));
    return true;
  }

  const request: SyncSetupImportRequest = { document: state.document, key: state.key, direction: state.direction };

  return {
    ...state,
    request,
    readFile,
    acceptScannedText,
    startScanning: () => setState((current) => ({ ...current, scanning: true, fileProblem: null })),
    stopScanning: () => setState((current) => ({ ...current, scanning: false })),
    setKey: (key: string) => setState((current) => ({ ...current, key })),
    setDirection: (direction: SyncDirection) => setState((current) => ({ ...current, direction })),
    // Forgets the document and the key: after a successful import, or to choose another document.
    reset: () => setState(initialState),
  };
}

export type SyncSetupImport = ReturnType<typeof useSyncSetupImport>;
