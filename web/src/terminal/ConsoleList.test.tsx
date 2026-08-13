import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConsoleList } from "./ConsoleList";
import type { TerminalSession } from "../api/integrations";

const live: TerminalSession = {
  id: "a", kind: "ssh", alias: "bastion", title: "bastion", startedAt: "2026-08-13T09:00:00Z",
};
const shell: TerminalSession = {
  id: "b", kind: "shell", title: "zsh", startedAt: "2026-08-13T09:01:00Z",
};
const dead: TerminalSession = {
  id: "c", kind: "ssh", alias: "db-primary", title: "db-primary", startedAt: "2026-08-13T09:02:00Z",
  exited: { code: 255, signal: "", at: "2026-08-13T09:02:01Z" },
};

function renderList(overrides: Partial<Parameters<typeof ConsoleList>[0]> = {}) {
  const props = {
    sessions: [live, shell, dead],
    selected: null,
    maxSessions: 50,
    busy: false,
    problem: "",
    onSelect: vi.fn(),
    onClose: vi.fn(),
    onOpenShell: vi.fn(),
    ...overrides,
  };
  render(<ConsoleList {...props} />);
  return props;
}

describe("ConsoleList", () => {
  // 終了したセッションは一覧に残る。最後の出力を読めるようにするためであり、
  // それが「接続できなかった理由」を読む唯一の場所になる。
  it("keeps exited sessions in the list and says so in words", () => {
    renderList();

    expect(screen.getByRole("button", { name: /^bastion/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^db-primary/ })).toBeInTheDocument();
    // 状態は点の色だけでなく語でも言う。色は目のためのもので、語は
    // それ以外のすべての人のためのものだ。
    expect(screen.getByRole("button", { name: /^db-primary/ })).toHaveTextContent("exited");
    expect(screen.getByRole("button", { name: /^bastion/ })).toHaveTextContent("ssh");
    expect(screen.getByRole("button", { name: /^zsh/ })).toHaveTextContent("shell");
  });

  it("reports selection and closing separately", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: /^bastion/ }));
    expect(props.onSelect).toHaveBeenCalledWith("a");

    await user.click(screen.getByRole("button", { name: "Close bastion" }));
    expect(props.onClose).toHaveBeenCalledWith("a");
  });

  // ローカルシェルの入口はここだけである。localhost はローカルシェルであって
  // ssh 接続ではないので、Home の接続一覧には出さない。
  it("is the only way in to a local shell", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Local shell" }));

    expect(props.onOpenShell).toHaveBeenCalledOnce();
  });

  // 上限に達したら開く操作を止め、その理由を書く。黙って何も起きないボタンは、
  // 壊れているのと区別が付かない。
  it("stops offering a new shell once the live limit is reached", () => {
    renderList({ sessions: [live, shell], maxSessions: 2 });

    expect(screen.getByRole("button", { name: "Local shell" })).toBeDisabled();
    expect(screen.getByText(/limit of 2 open consoles/)).toBeInTheDocument();
  });

  // 終了済みは生存上限に数えない。閉じた分だけ、また開けるようになる。
  it("does not count an exited session against the limit", () => {
    renderList({ sessions: [live, dead], maxSessions: 2 });

    expect(screen.getByRole("button", { name: "Local shell" })).toBeEnabled();
  });

  it("shows nothing to open and no list when there is no session", () => {
    renderList({ sessions: [] });

    expect(screen.getByText("No console is open.")).toBeInTheDocument();
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Local shell" })).toBeEnabled();
  });

  it("reports a refusal where the action was taken", () => {
    renderList({ problem: "No more consoles can be opened. Close one first." });

    expect(screen.getByRole("alert")).toHaveTextContent("No more consoles can be opened");
  });
});
