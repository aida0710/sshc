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
    onRename: vi.fn(async () => true),
    onDuplicate: vi.fn(),
    onReorder: vi.fn(),
    onOpenShell: vi.fn(),
    ...overrides,
  };
  render(<ConsoleList {...props} />);
  return props;
}

describe("ConsoleList", () => {
  // 終了したセッションは一覧に残り、位置も動かない。最後の出力を読めるように
  // するためであり、それが「接続できなかった理由」を読む唯一の場所になる。
  //
  // 状態でグループ分けはしない。2 行目が「状態 · 行き先」を語で言うので、
  // 見出しで囲う必要がない——点の色は目のためのもので、語はそれ以外の
  // すべての人のためのものだ。
  it("keeps every session in one flat list and says the state in words", () => {
    renderList();

    const rows = screen.getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("connected · bastion");
    // ローカルシェルの行き先は localhost である。種類を別に書かないのは、
    // 行き先がそれを言っているからだ。
    expect(rows[1]).toHaveTextContent("connected · localhost");
    expect(rows[2]).toHaveTextContent("exited 255 · db-primary");
  });

  // 同じ相手へ複数本開くと 2 行とも同じになる。だから改名がある。
  it("renames a session in place and leaves its destination alone", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
    await user.click(screen.getByRole("menuitem", { name: "Rename" }));
    const field = screen.getByRole("textbox", { name: "New name for bastion" });
    await user.clear(field);
    await user.type(field, "prod bastion{Enter}");

    expect(props.onRename).toHaveBeenCalledWith("a", "prod bastion");
  });

  it("keeps the old name when a rename is abandoned", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
    await user.click(screen.getByRole("menuitem", { name: "Rename" }));
    await user.type(screen.getByRole("textbox", { name: "New name for bastion" }), "x{Escape}");

    expect(props.onRename).not.toHaveBeenCalled();
  });

  it("opens another console to the same place", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
    await user.click(screen.getByRole("menuitem", { name: "Open another to the same place" }));

    expect(props.onDuplicate).toHaveBeenCalledWith("a");
  });

  // 並べ替えはドラッグでも行えるが、メニューにも置く。既存の drag and drop は
  // 矢印キーの経路を持たないので、ドラッグ専用にすると同じ穴を新設することになる。
  it("reorders from the menu so a keyboard can do it too", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Actions for zsh" }));
    await user.click(screen.getByRole("menuitem", { name: "Move up" }));

    expect(props.onReorder).toHaveBeenCalledWith(["b", "a", "c"]);
  });

  it("offers no move past either end", async () => {
    const user = userEvent.setup();
    renderList();

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));

    expect(screen.getByRole("menuitem", { name: "Move up" })).toBeDisabled();
    expect(screen.getByRole("menuitem", { name: "Move down" })).toBeEnabled();
  });

  it("reports selection and closing separately", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "bastion" }));
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

  // 上限をまだ知らないうちは、上限に達したことにしない。最初の一覧が届く前は
  // maxSessions が 0 であり、そのまま比べると入口が最初から無効になる。
  it("does not call itself full before the limit is known", () => {
    renderList({ sessions: [], maxSessions: 0 });

    expect(screen.getByRole("button", { name: "Local shell" })).toBeEnabled();
    expect(screen.queryByText(/limit of/)).not.toBeInTheDocument();
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
  // 転送はこのマシンにポートを開く。**開いていることが見えないまま開かない。**
  it("shows the forwards a console has open", () => {
    renderList({
      sessions: [{
        ...live,
        forwards: [
          { kind: "local", listen: "127.0.0.1:8080", to: "10.0.0.5:80", problem: "" },
          { kind: "dynamic", listen: "127.0.0.1:1080", to: "", problem: "" },
          { kind: "agent", listen: "", to: "", problem: "" },
        ],
      }],
    });

    expect(screen.getByText("forwarding 127.0.0.1:8080 → 10.0.0.5:80")).toBeVisible();
    expect(screen.getByText("SOCKS5 proxy on 127.0.0.1:1080")).toBeVisible();
    expect(screen.getByText("lending this agent to the remote")).toBeVisible();
  });

  // 開けなかったものは、開いたものと同じ場所で理由まで言う。
  it("says why a forward could not be opened", () => {
    renderList({
      sessions: [{
        ...live,
        forwards: [
          { kind: "local", listen: "127.0.0.1:8080", to: "10.0.0.5:80", problem: "address already in use" },
        ],
      }],
    });

    expect(screen.getByText("address already in use")).toBeVisible();
    expect(screen.queryByText(/forwarding 127/)).toBeNull();
  });
});
