import { useEffect, useMemo, useState } from "react";
import { terminalOverviewApi, type ConnectionHistory, type MachineStatus } from "../../api/terminalOverview";

// The machine is read again this often while the empty state is shown. Its CPU use is the average since the
// previous read, so this is also the span each value covers.
const machineReadInterval = 5000;
// The CPU uses kept for the small chart: the last minute and a quarter.
const cpuHistoryLength = 15;

export type TerminalOverview = {
  // null until read, and when the engine cannot give it (an engine older than the routes).
  machine: MachineStatus | null;
  // cpuHistory is the CPU use of the reads so far, oldest first.
  cpuHistory: readonly number[];
  history: ConnectionHistory | null;
};

// useTerminalOverview reads the machine and the connection history of the engine new shells open on, and keeps
// the machine up to date while the empty terminal is on screen: visible (no other page over it) and the page shown.
export function useTerminalOverview(runTarget: string | null, visible: boolean): TerminalOverview {
  const overviewApi = useMemo(() => terminalOverviewApi(runTarget), [runTarget]);
  const [machine, setMachine] = useState<MachineStatus | null>(null);
  const [cpuHistory, setCPUHistory] = useState<readonly number[]>([]);
  const [history, setHistory] = useState<ConnectionHistory | null>(null);

  useEffect(() => {
    let current = true;
    setHistory(null);
    overviewApi.connectionHistory().then(
      (read) => { if (current) setHistory(read); },
      () => undefined,
    );
    return () => { current = false; };
  }, [overviewApi]);

  // A new engine starts a new chart; the CPU uses of another engine are not this one's.
  useEffect(() => {
    setMachine(null);
    setCPUHistory([]);
  }, [overviewApi]);

  useEffect(() => {
    if (!visible) return;
    let current = true;
    let timer: ReturnType<typeof setInterval> | undefined;
    const read = () => {
      overviewApi.machineStatus().then(
        (status) => {
          if (!current) return;
          setMachine(status);
          const percent = status.cpuPercent;
          if (percent !== undefined) setCPUHistory((previous) => [...previous, percent].slice(-cpuHistoryLength));
        },
        () => { if (current) setMachine(null); },
      );
    };
    const start = () => {
      if (timer !== undefined) return;
      read();
      timer = setInterval(read, machineReadInterval);
    };
    const pause = () => {
      clearInterval(timer);
      timer = undefined;
    };
    const followVisibility = () => (document.hidden ? pause() : start());
    document.addEventListener("visibilitychange", followVisibility);
    if (!document.hidden) start();
    return () => {
      current = false;
      pause();
      document.removeEventListener("visibilitychange", followVisibility);
    };
  }, [overviewApi, visible]);

  return { machine, cpuHistory, history };
}
