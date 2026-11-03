import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { composerSendBlock, planComposerSend } from "./composerSend";

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

describe("composerSendBlock", () => {
  const session = { kind: "ssh", state: "connected" } as TerminalSession;
  const live = { phase: "live" } as const;

  it("lets a connected session with a live link send", () => {
    expect(composerSendBlock(session, live)).toBeNull();
  });

  it("asks to wait while the engine connects or reconnects, because it drops input until then", () => {
    expect(composerSendBlock({ ...session, state: "connecting" }, live)).toBe("wait-for-connection");
    expect(composerSendBlock({ ...session, state: "reconnecting" }, live)).toBe("wait-for-connection");
  });

  it("lets an answer through while the connection waits on an authentication prompt", () => {
    expect(composerSendBlock({ ...session, state: "connecting", awaitingPrompt: true }, live)).toBeNull();
    expect(composerSendBlock({ ...session, state: "reconnecting", awaitingPrompt: true }, live)).toBeNull();
    expect(composerSendBlock({ ...session, state: "connecting", awaitingPrompt: true }, { phase: "connecting", attempt: 1 })).toBe("wait-for-connection");
  });

  it("asks to wait while the browser's link to the engine is trying again", () => {
    expect(composerSendBlock(session, { phase: "connecting", attempt: 1 })).toBe("wait-for-connection");
    expect(composerSendBlock(session, { phase: "waiting", attempt: 2, seconds: 3 })).toBe("wait-for-connection");
  });

  it("asks to reconnect once an SSH session ended or the link stopped trying", () => {
    expect(composerSendBlock({ ...session, state: "exited" }, live)).toBe("reconnect-first");
    expect(composerSendBlock(session, { phase: "stopped", gone: false })).toBe("reconnect-first");
  });

  it("says there is nothing to reconnect to for a local shell that ended or a session the engine no longer has", () => {
    expect(composerSendBlock({ ...session, kind: "shell", state: "exited" }, live)).toBe("session-ended");
    expect(composerSendBlock(session, { phase: "stopped", gone: true })).toBe("session-ended");
  });
});
