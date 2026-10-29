import type { Translate } from "../i18n/context";
import { formatDuration } from "../ui/format";
import type { CommandBlock } from "./commandBlocks";

// commandClockMs is how often a running command's time is counted again: it is shown in whole seconds.
export const commandClockMs = 1_000;

// commandResult is how a command ended, or that it runs, with how long it took,
// for the command's details and its line kept above its output.
export function commandResult(block: CommandBlock, t: Translate, now: number): string {
  const end = block.status === "running" ? now : block.finishedAt;
  const duration = block.startedAt === null || end === null ? null : formatDuration(Math.max(0, Math.round((end - block.startedAt) / 1_000)), t);
  const result = block.status === "running"
    ? t("terminal.commandRunning")
    : block.status === "failed"
      ? t("terminal.commandFailed", { code: String(block.exitCode) })
      : block.status === "succeeded" ? t("terminal.commandSucceeded") : t("terminal.commandEnded");
  return duration === null ? result : `${result} · ${duration}`;
}

// commandName is the command as typed, or a word for it when the shell did not mark where it was typed.
export function commandName(block: CommandBlock, t: Translate): string {
  return block.command === "" ? t("terminal.commandUnnamed") : block.command;
}
