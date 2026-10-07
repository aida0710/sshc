import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { retryableTransferFailure, TransferPublicationUncertain, waitForTransferReconnect } from "./transferRecovery";

describe("transfer recovery", () => {
  it("retries only connection failures and stops on refusals or uncertain publication", () => {
    expect(retryableTransferFailure(new ApiError("sftp_connection_lost", 502, null))).toBe(true);
    for (const code of ["sftp_conflict", "sftp_permission_denied", "sftp_failed", "vault_locked"]) {
      expect(retryableTransferFailure(new ApiError(code, 409, null))).toBe(false);
    }
    expect(retryableTransferFailure(new TransferPublicationUncertain())).toBe(false);
    expect(retryableTransferFailure(new DOMException("Stopped", "AbortError"))).toBe(false);
  });

  it("cancels a reconnect wait immediately and releases its timer", async () => {
    vi.useFakeTimers();
    try {
      const controller = new AbortController();
      const waiting = waitForTransferReconnect(controller.signal, 30000);
      const stopped = expect(waiting).rejects.toMatchObject({ name: "AbortError" });
      controller.abort();
      await stopped;
      expect(vi.getTimerCount()).toBe(0);
    } finally { vi.useRealTimers(); }
  });
});
