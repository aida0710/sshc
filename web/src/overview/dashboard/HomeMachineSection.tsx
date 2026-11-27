import { MachineStatusLine } from "./MachineStatusLine";
import type { TerminalOverview } from "./useTerminalOverview";

// HomeMachineSection is the foot of Home: the machine new sessions open on (the run target; null is this machine).
// It is left out while the engine cannot give it.
export function HomeMachineSection({ overview, runTarget }: { overview: TerminalOverview; runTarget: string | null }) {
  if (overview.machine === null) return null;
  return (
    <div className="border-t border-line pt-3.5">
      <MachineStatusLine machine={overview.machine} cpuHistory={overview.cpuHistory} runTarget={runTarget} />
    </div>
  );
}
