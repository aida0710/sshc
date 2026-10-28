import { afterEach, describe, expect, it, vi } from "vitest";
import { saveFile } from "./saveFile";

afterEach(() => vi.restoreAllMocks());

describe("saveFile", () => {
  it("hands the blob to the browser's download under the name, and lets the URL go only when released", async () => {
    const created = vi.fn(() => "blob:sshc/1");
    const revoked = vi.fn();
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: created, revokeObjectURL: revoked }));
    const clicked = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      expect(this.download).toBe("notes.txt");
      expect(this.href).toBe("blob:sshc/1");
    });

    const save = await saveFile(new Blob(["text"]), "notes.txt");

    expect(clicked).toHaveBeenCalledOnce();
    expect(revoked).not.toHaveBeenCalled();
    save?.release();
    expect(revoked).toHaveBeenCalledExactlyOnceWith("blob:sshc/1");
  });
});
