import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import { sftpApi, type DirectoryComparison } from "./api";
import type { ComparisonMode } from "./contentToolTypes";
import type { SFTPLocation } from "./sftpLocation";
import { comparisonTransfers } from "./comparisonTransfers";
import { localHostAlias } from "./localHost";
import { sftpProblemText } from "./sftpProblemText";
import { sftpTransferManager } from "./transferManager";

export function useSFTPComparison({ left, right, onCopied }: {
  left: SFTPLocation;
  right: SFTPLocation;
  onCopied: () => void;
}) {
  const t = useTranslate();
  const readOnly = left.alias === localHostAlias || right.alias === localHostAlias;
  const [mode, setMode] = useState<ComparisonMode>("metadata");
  const [view, setView] = useState<{ mode: ComparisonMode; comparison: DirectoryComparison } | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(true);

  useEffect(() => {
    const controller = new AbortController();
    setBusy(true);
    setProblem("");
    setView(null);
    setSelected(new Set());
    sftpApi.compareDirectories({ left, right, mode, signal: controller.signal }).then((comparison) => {
      if (controller.signal.aborted) return;
      setView({ mode, comparison });
      setSelected(new Set(comparison.entries.filter((entry) => entry.status !== "same" && entry.status !== "unverified").map((entry) => entry.relativePath)));
    }).catch((error) => {
      if (!controller.signal.aborted) setProblem(sftpProblemText(t, error));
    }).finally(() => {
      if (!controller.signal.aborted) setBusy(false);
    });
    return () => controller.abort();
    // Locations are recreated by the workspace; their values identify a request.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [left.alias, left.path, right.alias, right.path, mode, t]);

  const comparison = view?.mode === mode ? view.comparison : null;
  const changes = comparison?.entries.filter((entry) => entry.status !== "same") ?? [];

  function toggle(relative: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(relative)) next.delete(relative);
      else next.add(relative);
      return next;
    });
  }

  async function copySelectedDifferences(direction: "left" | "right") {
    if (comparison === null || readOnly || busy) return;
    const transfers = comparisonTransfers({ comparison, selected, direction, left, right });
    if (transfers.length === 0) return;
    setBusy(true);
    setProblem("");
    try {
      await sftpTransferManager.addRemoteTransfers(transfers, "copy");
      onCopied();
    } catch (error) {
      setProblem(sftpProblemText(t, error));
      setBusy(false);
    }
  }

  return { comparison, changes, selected, problem, busy, readOnly, mode, setMode, toggle, copySelectedDifferences };
}
