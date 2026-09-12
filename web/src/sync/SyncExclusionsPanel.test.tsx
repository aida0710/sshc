import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { IntegrationsApi } from "../api/integrations";
import { LanguageProvider } from "../i18n/context";
import { SyncExclusionsPanel } from "./SyncExclusionsPanel";

function renderPanel(api: Partial<IntegrationsApi>) {
  render(
    <LanguageProvider initial="ja">
      <SyncExclusionsPanel api={api as IntegrationsApi} />
    </LanguageProvider>,
  );
}

describe("SyncExclusionsPanel", () => {
  it("loads on demand and saves exact path selections as shared rules", async () => {
    const api = {
      syncExclusions: vi.fn().mockResolvedValue({
        document: "*.tmp\nauthorized_keys\nknown_hosts\nknown_hosts.old\n",
        usingDefaults: true,
        candidates: [
          { path: "config", ignored: false },
          { path: "cache/session.tmp", ignored: true },
          { path: "authorized_keys", ignored: true },
          { path: "known_hosts", ignored: true },
          { path: "known_hosts.old", ignored: true },
        ],
      }),
      saveSyncExclusions: vi.fn().mockImplementation(async (document: string) => ({
        document,
        usingDefaults: false,
        candidates: [
          { path: "config", ignored: true },
          { path: "cache/session.tmp", ignored: true },
          { path: "authorized_keys", ignored: true },
          { path: "known_hosts", ignored: true },
          { path: "known_hosts.old", ignored: true },
        ],
      })),
    };
    renderPanel(api);

    expect(api.syncExclusions).not.toHaveBeenCalled();
    await userEvent.click(screen.getByText("同期するファイル"));
    expect(await screen.findByText("config")).toBeInTheDocument();
    for (const name of ["authorized_keys", "known_hosts", "known_hosts.old"]) {
      expect(screen.getByRole("checkbox", { name })).not.toBeChecked();
    }
    expect(screen.queryByText(/接続設定または鍵が除外されています/)).not.toBeInTheDocument();
    const config = screen.getByRole("checkbox", { name: "config" });
    expect(config).toBeChecked();
    await userEvent.click(config);
    expect(screen.getByText(/接続設定または鍵が除外されています/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "除外設定を保存" }));

    await waitFor(() =>
      expect(api.saveSyncExclusions).toHaveBeenCalledWith("*.tmp\nauthorized_keys\nknown_hosts\nknown_hosts.old\n/config\n"),
    );
  });

  it("warns before excluding connection material", async () => {
    const api = {
      syncExclusions: vi.fn().mockResolvedValue({
        document: "keys/**\n",
        usingDefaults: false,
        candidates: [{ path: "keys/work/id_ed25519", ignored: true }],
      }),
      saveSyncExclusions: vi.fn(),
    };
    renderPanel(api);
    await userEvent.click(screen.getByText("同期するファイル"));
    expect(
      await screen.findByText(/接続設定または鍵が除外されています/),
    ).toBeInTheDocument();
  });
});
