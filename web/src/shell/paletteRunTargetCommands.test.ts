import { describe, expect, it, vi } from "vitest";
import { defaultBindings } from "../keyconfig/bindings";
import { en } from "../i18n/messages/en";
import type { Translate } from "../i18n/context";
import { paletteRunTargetCommands } from "./paletteRunTargetCommands";

const t: Translate = (key, values) => en[key].replace("{name}", String(values?.name ?? ""));

describe("paletteRunTargetCommands", () => {
  it("names each remote session in the order of its list, with the key of its number and the one in use marked", () => {
    const commands = paletteRunTargetCommands({ t, bindings: defaultBindings, runTargets: [null, "bastion"], selectedRunTarget: "bastion", chooseRunTarget: vi.fn() });
    expect(commands.map(({ label, detail, keys }) => ({ label, detail, keys }))).toEqual([
      { label: "Remote session: localhost", detail: "", keys: ["Ctrl+1", "Meta+1"] },
      { label: "Remote session: bastion", detail: "In use", keys: ["Ctrl+2", "Meta+2"] },
    ]);
  });

  it("chooses the remote session of the item run", () => {
    const chooseRunTarget = vi.fn();
    const [thisMachine] = paletteRunTargetCommands({ t, bindings: defaultBindings, runTargets: [null, "bastion"], selectedRunTarget: "bastion", chooseRunTarget });
    void thisMachine?.run();
    expect(chooseRunTarget).toHaveBeenCalledWith(null);
  });

  it("lists nothing while there is only this machine to choose", () => {
    expect(paletteRunTargetCommands({ t, bindings: defaultBindings, runTargets: [null], selectedRunTarget: null, chooseRunTarget: vi.fn() })).toEqual([]);
  });
});
