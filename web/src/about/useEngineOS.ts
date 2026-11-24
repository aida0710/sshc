import { useEffect, useState } from "react";
import { aboutApi, type AboutApi } from "../api/about";
import { terminalOverviewApi } from "../api/terminalOverview";

// readEngineOS は、sshc エンジンが動く OS（"linux"、"windows" など。どちらも Go の GOOS）を読む。このマシン
// （runTarget null）は About から、リモートセッションの sshc エンジンは、取り次ぐマシンの状態（/terminal/machine）から読む。
// About は取り次がないため。
function readEngineOS(runTarget: string | null, about: AboutApi): Promise<string> {
  if (runTarget === null) return about.about().then((answer) => answer.os);
  return terminalOverviewApi(runTarget).machineStatus().then((machine) => machine.os);
}

// useEngineOS は、runTarget の sshc エンジンが動く OS。分かるまでと、読めなかったときは null。enabled が false なら
// 何も読まない。
export function useEngineOS({ enabled = true, runTarget = null, about = aboutApi }: {
  enabled?: boolean;
  runTarget?: string | null;
  about?: AboutApi;
} = {}): string | null {
  const [os, setOS] = useState<string | null>(null);
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    void readEngineOS(runTarget, about).then((read) => {
      if (!cancelled) setOS(read);
    }).catch(() => { if (!cancelled) setOS(null); });
    return () => { cancelled = true; };
  }, [enabled, runTarget, about]);
  return os;
}
