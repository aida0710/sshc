import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { SyncSetupExport } from "../api/sync";
import { saveFile } from "../ui/saveFile";
import { SyncSetupExportSection } from "./SyncSetupExportSection";

vi.mock("../ui/saveFile", () => ({ saveFile: vi.fn().mockResolvedValue(null) }));

const exported: SyncSetupExport = {
  document: '{"sshcSyncSettings":1,"sealed":"c3NoYw=="}',
  keyGenerated: true,
  qrRows: Array.from({ length: 21 }, () => "101010101010101010101"),
};

function renderSection(shown: SyncSetupExport | null) {
  render(<SyncSetupExportSection busy={false} exported={shown} onExport={() => {}} onClose={() => {}} />);
}

describe("SyncSetupExportSection", () => {
  it("saves the sealed document as a JSON file", async () => {
    renderSection(exported);

    await userEvent.click(screen.getByRole("button", { name: "Save to file" }));

    await waitFor(() => expect(saveFile).toHaveBeenCalledTimes(1));
    const [blob, name] = vi.mocked(saveFile).mock.calls[0]!;
    expect(name).toBe("sshc-sync-settings.json");
    expect(blob.type).toBe("application/json");
    expect(await blob.text()).toBe(exported.document);
  });

  it("warns that a key the person chose may be guessed from the exported settings", () => {
    renderSection({ ...exported, keyGenerated: false });

    expect(screen.getByRole("status")).toHaveTextContent("You chose this encryption key yourself.");
  });

  it("does not warn about a generated key", () => {
    renderSection(exported);

    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("offers the file alone when the settings are too long for a QR code", () => {
    const { qrRows: _tooLong, ...withoutQRCode } = exported;
    renderSection(withoutQRCode);

    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.getByText("These settings are too long for a QR code. Hand them over as a file.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save to file" })).toBeInTheDocument();
  });
});
