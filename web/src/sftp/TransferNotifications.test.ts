import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Translate } from "../i18n/context";
import type { TransferNotice } from "./transferManager";

const showBrowserNotification = vi.hoisted(() => vi.fn());
vi.mock("../terminal/terminalNotifications", () => ({ showBrowserNotification }));

const { notifyBackgroundTransfers } = await import("./TransferNotifications");

const notice: TransferNotice = {
  id: "job-1:completed:1",
  jobId: "job-1",
  status: "completed",
  name: "backup.tar",
  direction: "download",
  problem: "",
};

describe("transfer browser notifications", () => {
  beforeEach(() => showBrowserNotification.mockClear());

  it("delivers each transfer once while the tab is in the background", () => {
    const delivered = new Set<string>();
    const t: Translate = (key, values) => key === "sftp.manager.download"
      ? "Download"
      : `${values?.direction} completed: ${values?.name}`;

    notifyBackgroundTransfers([notice], delivered, t, true);
    notifyBackgroundTransfers([notice], delivered, t, true);

    expect(showBrowserNotification).toHaveBeenCalledOnce();
    expect(showBrowserNotification).toHaveBeenCalledWith({
      title: "sshc",
      body: "Download completed: backup.tar",
      tag: "sshc-transfer-job-1",
    });
  });

  it("marks foreground notices as seen without displaying them later", () => {
    const delivered = new Set<string>();
    const t: Translate = (key) => key;

    notifyBackgroundTransfers([notice], delivered, t, false);
    notifyBackgroundTransfers([notice], delivered, t, true);

    expect(showBrowserNotification).not.toHaveBeenCalled();
  });
});
