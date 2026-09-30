import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Translate } from "../i18n/context";
import { en } from "../i18n/messages";
import type { TransferNotice } from "./transferManager";

const showBrowserNotification = vi.hoisted(() => vi.fn());
vi.mock("../ui/browserNotifications", () => ({ showBrowserNotification }));

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

  it("says why a background transfer failed in words instead of its code", () => {
    const failed: TransferNotice = { ...notice, id: "job-1:failed:1", status: "failed", problem: "sftp_permission_denied" };
    const t: Translate = (key, values) =>
      en[key].replace(/\{(\w+)\}/g, (_, name: string) => String(values?.[name] ?? ""));

    notifyBackgroundTransfers([failed], new Set<string>(), t, true);

    expect(showBrowserNotification).toHaveBeenCalledWith(expect.objectContaining({
      body: "Download failed: backup.tar. Permission denied. Check the permissions on the host.",
    }));
  });
});
