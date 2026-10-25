import { useEffect, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { sftpApi, type BrowserSave } from "./api";
import { localHostAlias } from "./localHost";
import { sftpProblemText } from "./sftpProblemText";
import { sftpTransferManager } from "./transferManager";

// A size the transfer job learns from the engine once the download starts.
// The preview never listed the file, so it has no size to give.
const unknownTotalBytes = -1;

// usePreviewDownload saves the previewed file to the browser's machine. A
// file on an SSH host goes through the transfer queue, as a download from the
// file browser does, so it resumes and shows its progress there. A file on the
// engine's own disk is sent whole and saved at once.
//
// The browser may read a local save's blob after the save returns, so the
// blobs are released when the preview closes, not before.
export function usePreviewDownload(alias: string): { download: (path: string) => void; problem: string } {
  const t = useTranslate();
  const [problem, setProblem] = useState("");
  const saves = useRef<BrowserSave[]>([]);

  useEffect(() => () => {
    for (const save of saves.current) save.release();
    saves.current = [];
  }, []);

  function download(path: string) {
    setProblem("");
    void (async () => {
      try {
        if (alias !== localHostAlias) {
          await sftpTransferManager.addDownload(alias, path, "file", unknownTotalBytes);
          return;
        }
        const save = await sftpApi.saveDownload(path, false, [await sftpApi.downloadLocalFile(path)]);
        if (save !== null) saves.current.push(save);
      } catch (error) {
        setProblem(sftpProblemText(t, error));
      }
    })();
  }

  return { download, problem };
}
