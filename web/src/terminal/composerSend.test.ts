import { describe, expect, it } from "vitest";
import { planComposerSend } from "./composerSend";

describe("planComposerSend", () => {
  it("presses Enter when nothing was typed", () => {
    expect(planComposerSend("")).toEqual({ kind: "enter" });
  });

  it("sends one line as a paste followed by Enter", () => {
    expect(planComposerSend("ビルドして確かめて")).toEqual({ kind: "paste", text: "ビルドして確かめて" });
  });

  it("asks first when the text has a line break, as a paste would", () => {
    expect(planComposerSend("一行目\n二行目")).toEqual({ kind: "confirm", text: "一行目\n二行目" });
  });

  it("asks first when the text has a control character", () => {
    expect(planComposerSend("echo \u001b[31mred")).toEqual({ kind: "confirm", text: "echo \u001b[31mred" });
  });
});
