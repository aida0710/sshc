import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it } from "vitest";
import { attachVisibilityReportQuestions } from "./visibilityReports";

const cleanups: (() => void)[] = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

describe("visibility report questions in the browser", () => {
  it("leaves DECRQM 2033 to the engine and still answers the modes xterm.js knows", async () => {
    const view = new Terminal({ allowProposedApi: true });
    const questions = attachVisibilityReportQuestions(view.parser);
    cleanups.push(() => {
      questions.dispose();
      view.dispose();
    });
    const answers: string[] = [];
    view.onData((data) => answers.push(data));
    const write = (data: string) => new Promise<void>((resolve) => view.write(data, resolve));

    // Without the handler xterm.js answers "\x1b[?2033;0$y", that it does not know the mode.
    await write("\u001b[?2033$p");
    await write("\u001b[?1$p");

    expect(answers).toEqual(["\u001b[?1;2$y"]);
  });
});
