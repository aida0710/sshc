import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { OrphanPanel } from "./OrphanPanel";
import type { HostEntry, HostMetadata, Metadata } from "../api/config";

const hosts: HostEntry[] = [
  {
    identity: { path: "config", alias: "bastion" },
    file: { path: "config", absolute: "/home/tester/.ssh/config" },
    line: 1,
    patterns: ["bastion"],
    editable: true,
  },
  {
    identity: { path: "config", alias: "nas" },
    file: { path: "config", absolute: "/home/tester/.ssh/config" },
    line: 8,
    patterns: ["nas"],
    editable: true,
  },
];

function metadataWithOrphan(): Metadata {
  return {
    schemaVersion: 10, presets: [{ id: "default" }],
    hosts: [
      {
        identity: { path: "config", alias: "old-build" },
        tags: ["ci"],
        colour: "#f97316",
        orphan: true,
      },
      { identity: { path: "config", alias: "bastion" }, tags: ["kept"] },
    ],
  };
}

describe("OrphanPanel", () => {
  it("renders nothing when no entry is orphaned", () => {
    const metadata: Metadata = {
      schemaVersion: 10, presets: [{ id: "default" }],
      hosts: [{ identity: { path: "config", alias: "bastion" }, tags: ["kept"] }],
    };
    const { container } = render(<OrphanPanel metadata={metadata} hosts={hosts} onSave={vi.fn()} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("says what the stranded settings hold, so discarding them is an informed choice", () => {
    render(<OrphanPanel metadata={metadataWithOrphan()} hosts={hosts} onSave={vi.fn()} />);

    expect(screen.getByText("old-build in config")).toBeInTheDocument();
    expect(screen.getByText(/tags ci/)).toBeInTheDocument();
    expect(screen.getByText(/#f97316/)).toBeInTheDocument();
  });

  it("re-points the entry at the host the user chose and drops the server's flag", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<OrphanPanel metadata={metadataWithOrphan()} hosts={hosts} onSave={onSave} />);

    await user.selectOptions(screen.getByLabelText("Re-associate old-build with"), "config\u0000nas");
    await user.click(screen.getByRole("button", { name: "Re-associate old-build" }));

    expect(onSave).toHaveBeenCalledTimes(1);
    const [base, moved] = onSave.mock.calls[0]! as [HostMetadata, HostMetadata];
    expect(base.identity).toEqual({ path: "config", alias: "old-build" });
    expect(moved).toMatchObject({ identity: { path: "config", alias: "nas" }, tags: ["ci"], colour: "#f97316" });
    expect(moved).not.toHaveProperty("orphan");
  });

  it("refuses a target that already has its own settings instead of merging them", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<OrphanPanel metadata={metadataWithOrphan()} hosts={hosts} onSave={onSave} />);

    await user.selectOptions(screen.getByLabelText("Re-associate old-build with"), "config\u0000bastion");
    await user.click(screen.getByRole("button", { name: "Re-associate old-build" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("bastion already has its own settings");
    expect(onSave).not.toHaveBeenCalled();
  });

  it("asks for a target rather than picking one", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<OrphanPanel metadata={metadataWithOrphan()} hosts={hosts} onSave={onSave} />);

    await user.click(screen.getByRole("button", { name: "Re-associate old-build" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Choose the connection this note belongs to");
    expect(onSave).not.toHaveBeenCalled();
  });

  it("discards the note when that is what the user wants", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<OrphanPanel metadata={metadataWithOrphan()} hosts={hosts} onSave={onSave} />);

    await user.click(screen.getByRole("button", { name: "Discard old-build settings" }));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ identity: { path: "config", alias: "old-build" } }), null);
  });
});
