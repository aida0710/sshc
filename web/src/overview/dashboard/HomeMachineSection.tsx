import { ConnectionActivityPanel } from "./ConnectionActivityPanel";
import { MachineStatusLine } from "./MachineStatusLine";
import type { TerminalOverview } from "./useTerminalOverview";

// HomeMachineSection is the foot of Home: the machine new sessions open on (the run target; null is this machine)
// and how often connections were opened from it. Each part is left out while the engine cannot give it.
export function HomeMachineSection({ overview, runTarget }: { overview: TerminalOverview; runTarget: string | null }) {
  const hours = overview.history?.hours ?? [];
  if (overview.machine === null && hours.length === 0) return null;
  return (
    <div className="flex flex-col gap-3 border-t border-line pt-3.5">
      {overview.machine === null ? null : <MachineStatusLine machine={overview.machine} cpuHistory={overview.cpuHistory} runTarget={runTarget} />}
      {hours.length === 0 ? null : <ConnectionActivityPanel hours={hours} />}
    </div>
  );
}
