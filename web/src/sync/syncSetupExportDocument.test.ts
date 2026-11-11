import { describe, expect, it } from "vitest";
import { looksLikeSyncSetupExport } from "./syncSetupExportDocument";

describe("looksLikeSyncSetupExport", () => {
  it("accepts the document the engine exports", () => {
    expect(looksLikeSyncSetupExport('{"sshcSyncSettings":1,"sealed":"c3NoYw=="}\n')).toBe(true);
  });

  it.each([
    ["another QR code", "https://example.com/"],
    ["other JSON", '{"endpoint":"https://s3.example.com"}'],
    ["a JSON value that is not an object", "null"],
    ["a document longer than the engine accepts", `{"sshcSyncSettings":1,"sealed":"${"A".repeat(16 * 1024)}"}`],
  ])("passes over %s", (_name, text) => {
    expect(looksLikeSyncSetupExport(text)).toBe(false);
  });
});
