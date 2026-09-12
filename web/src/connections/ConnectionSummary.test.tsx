import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ConnectionSavedState } from "./connectionSavedState";
import { ConnectionSummary } from "./ConnectionSummary";

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
      sizeBytes: 444,
      references: [],
      notes: [],
    }],
  },
  vault: {
    status: "ready",
    value: {
      exists: true,
      unlocked: true,
      aliases: ["bastion"],
      dedicatedKeyPassphrases: ["id_work"],
      minPassphraseLength: 12,
    },
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
  it("renders a direct key and legacy password assignment only as pending cleanup", () => {
    render(
      <ConnectionSummary
        state={state}
        dirty={false}
        refreshing={false}
        onConnect={vi.fn()}
        connecting={false}
        onToggleManage={vi.fn()}
        managing={false}
      />,
    );

    expect(screen.getByRole("heading", { name: "bastion" })).toBeInTheDocument();
    expect(screen.getByText("ops@203.0.113.10:2222")).toBeInTheDocument();
    expect(screen.getByText("id_work · SHA256:work")).toBeInTheDocument();
    expect(screen.getByText("Saved only for this key")).toBeInTheDocument();
    expect(screen.queryByText("Saved password: office")).not.toBeInTheDocument();
    expect(screen.getByText(/is not used and will be unassigned/i)).toBeInTheDocument();
    expect(screen.getByText("work")).toBeInTheDocument();
  });

  it("does not claim a password cleanup when credential metadata is unavailable", () => {
    render(
      <ConnectionSummary
        state={{ ...state, credentials: { status: "failed" } }}
        dirty={false}
        refreshing={false}
        onConnect={vi.fn()}
        connecting={false}
        onToggleManage={vi.fn()}
        managing={false}
      />,
    );

    expect(screen.queryByText(/is not used and will be unassigned/i)).not.toBeInTheDocument();
  });

  it("keeps configured key details without an empty account-password field", () => {
    render(
      <ConnectionSummary
        state={{
          ...state,
          credentials: { status: "ready", value: [] },
          vault: {
            status: "ready",
            value: {
              exists: true,
              unlocked: true,
              aliases: [],
              dedicatedKeyPassphrases: ["id_work"],
              minPassphraseLength: 12,
            },
          },
        }}
        dirty={false}
        refreshing={false}
        onConnect={vi.fn()}
        connecting={false}
        onToggleManage={vi.fn()}
        managing={false}
      />,
    );

    expect(screen.getByText("work")).toBeInTheDocument();
    expect(screen.getByText("id_work · SHA256:work")).toBeInTheDocument();
    expect(screen.getByText("Saved only for this key")).toBeInTheDocument();
    expect(screen.queryByText("Account password")).not.toBeInTheDocument();
  });

  it("keeps the default authentication summary concise and the main action available", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    render(
      <ConnectionSummary
        state={{
          ...state,
          detail: {
            ...state.detail,
            form: {
              ...state.detail.form,
              entry: { ...state.detail.form.entry, group: "" },
              fields: state.detail.form.fields.filter((field) => field.keyword !== "IdentityFile"),
            },
          },
          credentials: { status: "ready", value: [] },
          vault: {
            status: "ready",
            value: {
              exists: true,
              unlocked: true,
              aliases: [],
              dedicatedKeyPassphrases: [],
              minPassphraseLength: 4,
            },
          },
        }}
        dirty={false}
        refreshing={false}
        onConnect={onConnect}
        connecting={false}
        onToggleManage={vi.fn()}
        managing={false}
      />,
    );

    expect(screen.getByRole("region", { name: "bastion" })).toBeInTheDocument();
    expect(screen.getByText("SSH agent or inherited keys")).toBeInTheDocument();
    expect(screen.queryByText("Saved connection")).not.toBeInTheDocument();
    expect(screen.queryByText("Saved")).not.toBeInTheDocument();
    expect(screen.queryByText("Ungrouped")).not.toBeInTheDocument();
    expect(screen.queryByText("Key passphrase")).not.toBeInTheDocument();
    expect(screen.queryByText("Account password")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(onConnect).toHaveBeenCalledOnce();
  });

  it("keeps committed text while disabling saved-state actions for a draft", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    const onToggleManage = vi.fn();
    render(
      <ConnectionSummary
        state={state}
        dirty
        refreshing={false}
        onConnect={onConnect}
        connecting={false}
        onToggleManage={onToggleManage}
        managing={false}
      />,
    );

    expect(screen.getByText("ops@203.0.113.10:2222")).toBeInTheDocument();
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    expect(screen.getByText(/Save or discard this draft/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "More connection actions" }));
    expect(onConnect).not.toHaveBeenCalled();
    expect(onToggleManage).toHaveBeenCalledOnce();
  });

  it("keeps Connect unavailable when this platform cannot launch a terminal", () => {
    render(
      <ConnectionSummary
        state={state}
        dirty={false}
        refreshing={false}
        connectAvailable={false}
        onConnect={vi.fn()}
        connecting={false}
        onToggleManage={vi.fn()}
        managing={false}
      />,
    );

    expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
  });
});
