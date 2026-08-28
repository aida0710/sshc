import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LanguageProvider } from "../i18n/context";
import type { PullResponse, PushResult, SyncOperation } from "../api/integrations";
import { SyncResultCard, formatBytes } from "./SyncResultCard";

const summary = {
  createdAt: "2026-08-12T01:30:00Z",
  fileCount: 12,
  sourceBytes: 4_800_000,
  snapshotBytes: 1_900_000,
};

const push: PushResult = {
  summary,
  objectCount: 2,
  uploadedBytes: 3_800_000,
  completedAt: "2026-08-12T01:30:03Z",
};

const pull: PullResponse = {
  applied: false,
  summary,
  downloadedBytes: 1_900_000,
  completedAt: "2026-08-12T01:31:00Z",
  written: ["config", "connections/work/lon.conf", "keys/work/id_ed25519"],
  removed: ["connections/old.conf"],
  conflicts: [{ path: "config", changedHere: true, changedThere: true }],
  remoteETag: '"generation-1"',
  remoteRevision: "a".repeat(64),
};

function renderJapanese(view: Parameters<typeof SyncResultCard>[0]["view"]) {
  return render(
    <LanguageProvider initial="ja">
      <SyncResultCard view={view} />
    </LanguageProvider>,
  );
}

describe("formatBytes", () => {
  it.each([
    [0, "0 B"],
    [999, "999 B"],
    [1_000, "1 kB"],
    [1_500, "1.5 kB"],
    [1_000_000, "1 MB"],
    [4_800_000, "4.8 MB"],
  ])("formats %i bytes with decimal units", (bytes, expected) => {
    expect(formatBytes(bytes, "en")).toBe(expected);
  });
});

describe("SyncResultCard", () => {
  it("separates source, one encrypted object, and both uploaded objects", () => {
    renderJapanese({ kind: "push", result: push });

    expect(screen.getByRole("heading", { name: "今回の送信" })).toBeInTheDocument();
    expect(screen.getByText("12ファイル・4.8 MB")).toBeInTheDocument();
    expect(screen.getByText("暗号化スナップショット 1.9 MB")).toBeInTheDocument();
    expect(screen.getByText("S3転送 3.8 MB（2オブジェクト、履歴＋現在版）")).toBeInTheDocument();
    expect(screen.getByText(/スナップショット作成/)).toBeInTheDocument();
    expect(screen.getByText(/操作完了/)).toBeInTheDocument();
  });

  it("shows preview download, expanded source size, and all change counts", () => {
    renderJapanese({ kind: "preview", result: pull });

    expect(screen.getByRole("heading", { name: "受信内容の確認" })).toBeInTheDocument();
    expect(screen.getByText("1.9 MB取得・展開後4.8 MB")).toBeInTheDocument();
    expect(screen.getByText("書き込み3件・削除1件・競合1件")).toBeInTheDocument();
  });

  it("says an apply re-downloaded and applied a snapshot without claiming a full restore", () => {
    const applied = { ...pull, applied: true, conflicts: [] };
    renderJapanese({ kind: "apply", result: applied });

    expect(screen.getByRole("heading", { name: "適用結果" })).toBeInTheDocument();
    expect(screen.getByText(/のスナップショットを適用/)).toBeInTheDocument();
    expect(screen.getByText("適用時に再取得 1.9 MB")).toBeInTheDocument();
    expect(document.body.textContent).not.toContain("完全復元");
  });

  it("labels a persisted operation as the previous success", () => {
    const operation: SyncOperation = {
      kind: "push",
      summary,
      objectCount: 2,
      uploadedBytes: 3_800_000,
      completedAt: "2026-08-12T01:30:03Z",
    };
    renderJapanese({ kind: "previous", operation });

    expect(screen.getByRole("heading", { name: "前回の成功" })).toBeInTheDocument();
    expect(screen.getByText("S3転送 3.8 MB（2オブジェクト、履歴＋現在版）")).toBeInTheDocument();
  });
});
