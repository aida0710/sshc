import { createRef } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SFTPMetadataActionDialog } from "./SFTPMetadataActionDialog";
import type { SFTPMetadataActionsModel } from "./useSFTPMetadataActions";

function actions(intent: SFTPMetadataActionsModel["intent"]): SFTPMetadataActionsModel {
  return { intent, directory: "/srv", acting: false, problem: "", submit: vi.fn(async () => undefined), ask: vi.fn(), cancel: vi.fn() };
}
const entry = { name: "file", path: "/srv/file", type: "file" as const, size: 0, mode: "-rw-r--r--", modifiedAt: "2026-10-07T00:00:00Z", revision: "rev", uid: 1000, gid: 100 };

describe("SFTP metadata dialogs", () => {
  it("shows the current owner IDs and refuses names, negative and overflowing IDs", async () => {
    const model = actions({ kind: "ownership", entry });
    render(<SFTPMetadataActionDialog actions={model} returnFocusRef={createRef()} />);
    expect(screen.getByText("Current: UID 1000 / GID 100")).toBeInTheDocument();
    const uid = screen.getByLabelText("Owner (UID)");
    for (const invalid of ["root", "-1", "4294967296"]) {
      await userEvent.clear(uid);
      await userEvent.type(uid, invalid);
      await userEvent.click(screen.getByRole("button", { name: "Apply" }));
      expect(screen.getByRole("alert")).toHaveTextContent("UID and GID must be integers");
    }
    expect(model.submit).not.toHaveBeenCalled();
    await userEvent.clear(uid);
    await userEvent.type(uid, "0");
    await userEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(model.submit).toHaveBeenCalledWith({ name: "", target: "", uid: 0, gid: 100 });
  });

  it("allows a relative dangling link target while rejecting an invalid new name", async () => {
    const model = actions({ kind: "createSymlink" });
    render(<SFTPMetadataActionDialog actions={model} returnFocusRef={createRef()} />);
    await userEvent.type(screen.getByLabelText("Name"), "../escape");
    await userEvent.type(screen.getByLabelText("Points to"), "../not-created-yet");
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(model.submit).not.toHaveBeenCalled();
    await userEvent.clear(screen.getByLabelText("Name"));
    await userEvent.type(screen.getByLabelText("Name"), "new-link");
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(model.submit).toHaveBeenCalledWith({ name: "new-link", target: "../not-created-yet", uid: 0, gid: 0 }));
  });
});
