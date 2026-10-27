import { describe, expect, it } from "vitest";
import { planComposerSend } from "./composerSend";

const plain = { bracketedPaste: false };
const bracketed = { bracketedPaste: true };

describe("planComposerSend", () => {
  it("presses Enter when nothing was typed", () => {
    expect(planComposerSend("", plain)).toEqual({ kind: "enter" });
  });

  it("sends one line as a paste followed by Enter", () => {
    expect(planComposerSend("ビルドして確かめて", plain)).toEqual({ kind: "paste", text: "ビルドして確かめて" });
  });

  it("asks first when the text has a line break, as a paste would", () => {
    expect(planComposerSend("一行目\n二行目", plain)).toEqual({ kind: "confirm", text: "一行目\n二行目" });
  });

  it("sends lines without asking when the program takes the text as one bracketed paste", () => {
    expect(planComposerSend("一行目\n二行目", bracketed)).toEqual({ kind: "paste", text: "一行目\n二行目" });
  });

  it("asks first when the text has a control character, even in a bracketed paste", () => {
    expect(planComposerSend("echo \u001b[31mred", bracketed)).toEqual({ kind: "confirm", text: "echo \u001b[31mred" });
  });
});
