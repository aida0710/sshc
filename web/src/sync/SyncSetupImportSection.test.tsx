import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SyncSetupImportSection } from "./SyncSetupImportSection";
import { useSyncSetupImport } from "./useSyncSetupImport";

// The camera is replaced by a button that "sees" the QR codes a test names, in order.
let codesInView: string[] = [];
vi.mock("../ui/QRCodeScanner", () => ({
  QRCodeScanner: ({ accept, onCancel }: { accept: (text: string) => boolean; onCancel: () => void }) => (
    <>
      <button onClick={() => codesInView.forEach((code) => accept(code))}>Camera sees the codes</button>
      <button onClick={onCancel}>Stop scanning</button>
    </>
  ),
}));

const exportedDocument = '{"sshcSyncSettings":1,"sealed":"c3NoYw=="}';

function Section({ onImport = () => {} }: { onImport?: () => void }) {
  const form = useSyncSetupImport();
  return <SyncSetupImportSection busy={false} form={form} onImport={onImport} />;
}

function offerCamera() {
  vi.stubGlobal("navigator", { ...navigator, mediaDevices: { getUserMedia: vi.fn() } });
}

afterEach(() => {
  codesInView = [];
  vi.unstubAllGlobals();
});

describe("SyncSetupImportSection", () => {
  it("refuses a file that is not exported sync settings and keeps asking for one", async () => {
    const { container } = render(<Section />);

    await userEvent.upload(
      container.querySelector<HTMLInputElement>('input[type="file"]')!,
      new File(['{"endpoint":"https://s3.example.com"}'], "other.json"),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent("This file is not sync settings exported by sshc.");
    expect(screen.queryByLabelText("Encryption key")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Choose file" })).toBeInTheDocument();
  });

  it("takes the settings from a scanned QR code, passing over other codes in view", async () => {
    offerCamera();
    codesInView = ["https://example.com/", exportedDocument];
    render(<Section />);

    await userEvent.click(screen.getByRole("button", { name: "Scan QR code" }));
    await userEvent.click(screen.getByRole("button", { name: "Camera sees the codes" }));

    expect(await screen.findByLabelText("Encryption key")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Camera sees the codes" })).not.toBeInTheDocument();
  });

  it("goes back to the choice of file or camera when scanning is stopped", async () => {
    offerCamera();
    render(<Section />);

    await userEvent.click(screen.getByRole("button", { name: "Scan QR code" }));
    await userEvent.click(screen.getByRole("button", { name: "Stop scanning" }));

    expect(screen.getByRole("button", { name: "Choose file" })).toBeInTheDocument();
  });

  it("offers only the file where the page cannot open a camera", () => {
    render(<Section />);

    expect(screen.getByRole("button", { name: "Choose file" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Scan QR code" })).not.toBeInTheDocument();
  });

  it("does not import until an encryption key is typed, and lets another document be chosen", async () => {
    const onImport = vi.fn();
    const { container } = render(<Section onImport={onImport} />);
    await userEvent.upload(
      container.querySelector<HTMLInputElement>('input[type="file"]')!,
      new File([exportedDocument], "sshc-sync-settings.json"),
    );

    expect(await screen.findByRole("button", { name: "Import" })).toBeDisabled();
    await userEvent.type(screen.getByLabelText("Encryption key"), "AB12-CD34-EF56-GH78-JK90-MN12{Enter}");
    expect(onImport).toHaveBeenCalledTimes(1);

    await userEvent.click(screen.getByRole("button", { name: "Choose other settings" }));
    expect(screen.getByRole("button", { name: "Choose file" })).toBeInTheDocument();
  });
});
