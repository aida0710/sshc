import { useEffect, useState } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { sftpApi, type RemoteEntry } from "./api";
import { sftpProblemText } from "./sftpProblemText";

export type TextComparisonSide = { alias: string; entry?: RemoteEntry | undefined };
type TextComparison = { original: string; modified: string };

export function useSFTPTextComparison({ left, right }: { left: TextComparisonSide; right: TextComparisonSide }) {
  const t = useTranslate();
  const [comparison, setComparison] = useState<TextComparison | null>(null);
  const [problem, setProblem] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    setComparison(null); setProblem("");
    const read = async (side: TextComparisonSide) => side.entry === undefined ? "" : (await sftpApi.readText(side.alias, side.entry.path, { expectedRevision: side.entry.revision, signal: controller.signal })).contents;
    void Promise.all([read(left), read(right)]).then(([original, modified]) => {
      if (!controller.signal.aborted) setComparison({ original, modified });
    }).catch((error) => {
      if (controller.signal.aborted) return;
      controller.abort();
      const code = failureCode(error);
      setProblem(code === "sftp_not_utf8" ? t("sftp.binaryHint") : code === "sftp_text_too_large" ? t("sftp.tooLargeHint") : sftpProblemText(t, error));
    });
    return () => controller.abort();
    // A rerender of the parent does not retire reads for the same files.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [left.alias, left.entry?.path, left.entry?.revision, right.alias, right.entry?.path, right.entry?.revision, t]);
  return { comparison, problem };
}
