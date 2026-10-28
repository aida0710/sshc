import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { vaultStatus } from "../testing/vaultStatus";
import type { ConnectionSavedState } from "./connectionSavedState";
import { ConnectionSummary } from "./ConnectionSummary";
import { PageChromeContext } from "../ui/pageChrome";

const state: ConnectionSavedState = {
  detail: {
    form: {
      entry: {
        identity: { path: "config", alias: "bastion" },
        file: { path: "config", absolute: "/home/tester/.ssh/config" },
        line: 1,
        patterns: ["bastion"],
        editable: true,
        group: "work",
      },
      fields: [
        { line: 2, keyword: "HostName", values: ["203.0.113.10"], category: "basic", editable: true },
        { line: 3, keyword: "User", values: ["ops"], category: "basic", editable: true },
        { line: 4, keyword: "Port", values: ["2222"], category: "basic", editable: true },
        { line: 5, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
      ],
      raw: "Host bastion\n",
      comment: "",
      commentLines: 0,
    },
    metadata: { identity: { path: "config", alias: "bastion" } },
    effective: { alias: "bastion", entries: [] },
    file: {
      file: { path: "config", absolute: "/home/tester/.ssh/config" },
      contents: "Host bastion\n",
      digest: "digest",
      editable: true,
      exists: true,
    },
  },
  keys: {
    status: "ready",
    value: [{
      id: "0123456789abcdef0123456789abcdef",
      relativePath: "id_work",
      kind: "private_key",
      container: "OPENSSH PRIVATE KEY",
      algorithm: "ed25519",
      keyType: "ssh-ed25519",
      bits: 256,
      encrypted: true,
      fingerprint: "SHA256:work",
      comment: "work",
      permission: "0600",
      permissionRisk: false,
      references: [],
      notes: [],
    }],
  },
  vault: {
    status: "ready",
    value: vaultStatus({ aliases: ["bastion"], dedicatedKeyPassphrases: ["id_work"] }),
  },
  credentials: {
    status: "ready",
    value: [{ kind: "password", name: "office", uses: ["bastion"], hosts: ["bastion"] }],
  },
  eligibility: {
    status: "ready",
    value: { alias: "bastion", storable: true, blockers: [], warnings: [] },
  },
};

describe("ConnectionSummary", () => {
  it("leaves the password cleanup to the authentication part of the Basic tab", () => {
    render(
      <ConnectionSummary
        state={state}
        dirty={false}
        refreshState="idle"
        onConnect={vi.fn()}
        connecting={false}
        actions={[]}
      />,
    );

    expect(screen.getByRole("heading", { name: "bastion" })).toBeInTheDocument();
    expect(screen.getByText("ops@203.0.113.10:2222")).toBeInTheDocument();
    expect(screen.queryByText(/is not used and will be unassigned/i)).not.toBeInTheDocument();
  });

  it("puts Connect and the menu in the phone's top bar when it has one", () => {
    const topBar = document.createElement("div");
    document.body.append(topBar);
    render(
      <PageChromeContext.Provider value={{ inspectorToggle: null, phoneActionsSlot: topBar, phoneBackSlot: null }}>
        <ConnectionSummary state={state} dirty={false} refreshState="idle" onConnect={vi.fn()} connecting={false} actions={[]} />
      </PageChromeContext.Provider>,
    );

    expect(within(topBar).getByRole("button", { name: "Connect" })).toBeInTheDocument();
    expect(within(topBar).getByRole("button", { name: "Actions for this connection" })).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "bastion" })).queryByRole("button", { name: "Connect" })).toBeNull();
    topBar.remove();
  });

  it("keeps only the name, address, Connect and the menu in the header", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    const onRename = vi.fn();
    render(
      <ConnectionSummary
        state={state}
        dirty={false}
        refreshState="idle"
        onConnect={onConnect}
        connecting={false}
        actions={[{ label: "Rename", onSelect: onRename }]}
      />,
    );

    expect(screen.getByRole("region", { name: "bastion" })).toBeInTheDocument();
    expect(screen.queryByText("id_work · SHA256:work")).not.toBeInTheDocument();
    expect(screen.queryByText("work")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(onConnect).toHaveBeenCalledOnce();
    await user.click(screen.getByRole("button", { name: "Actions for this connection" }));
    await user.click(screen.getByRole("menuitem", { name: "Rename" }));
    expect(onRename).toHaveBeenCalledOnce();
  });
  it("keeps committed text while disabling saved-state actions for a draft", () => {
    const onConnect = vi.fn();
    render(
      <ConnectionSummary
        state={state}
        dirty
        refreshState="idle"
        onConnect={onConnect}
        connecting={false}
        actions={[]}
      />,
    );

    expect(screen.getByText("ops@203.0.113.10:2222")).toBeInTheDocument();
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    expect(screen.getByText(/Save or discard this draft/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
    expect(onConnect).not.toHaveBeenCalled();
  });

  it("keeps Connect unavailable when this platform cannot launch a terminal", () => {
    render(
      <ConnectionSummary
        state={state}
        dirty={false}
        refreshState="idle"
        connectAvailable={false}
        onConnect={vi.fn()}
        connecting={false}
        actions={[]}
      />,
    );

    expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
  });

  it("says it is reloading while the saved connection is being reloaded", () => {
    render(
      <ConnectionSummary
        state={state}
        dirty={false}
        refreshState="refreshing"
        onConnect={vi.fn()}
        connecting={false}
        actions={[]}
      />,
    );

    expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Connect" })).toHaveAccessibleDescription(
      "Reloading the saved connection. Actions will be available when it finishes.",
    );
  });

  it("does not say it is reloading once the reload has failed, and keeps Connect unavailable", () => {
    render(
      <ConnectionSummary
        state={state}
        dirty={false}
        refreshState="failed"
        onConnect={vi.fn()}
        connecting={false}
        actions={[]}
      />,
    );

    expect(screen.queryByText(/Reloading the saved connection/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Connect" })).toHaveAccessibleDescription(
      "Reload the saved connection before using it.",
    );
  });
});
