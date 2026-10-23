import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { LanguageProvider } from "../i18n/context";
import { sftpApi } from "./api";
import { FilePreviewDialog } from "./FilePreviewDialog";

vi.mock("./api", () => ({ sftpApi: { previewFile: vi.fn(), readText: vi.fn() } }));

function renderDialog(path: string) {
  render(
    <LanguageProvider>
      <FilePreviewDialog alias="web" path={path} onClose={() => undefined} />
    </LanguageProvider>,
  );
}

describe("FilePreviewDialog", () => {
  beforeEach(() => vi.resetAllMocks());

  it("shows the image read from the alias", async () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue({
      contentType: "image/png",
      revision: "r1",
      blob: new Blob([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], { type: "image/png" }),
    });
    renderDialog("/tmp/shot.png");

    const image = await screen.findByRole("img", { name: "shot.png" });
    expect(image.getAttribute("src")).toMatch(/^data:image\/png;base64,/u);
    expect(sftpApi.previewFile).toHaveBeenCalledWith("web", "/tmp/shot.png");
  });

  it("shows text when the bytes are not an image", async () => {
    vi.mocked(sftpApi.previewFile).mockRejectedValue(new ApiError("sftp_preview_type", 415, null));
    vi.mocked(sftpApi.readText).mockResolvedValue({ contents: "hello", revision: "r1" } as Awaited<ReturnType<typeof sftpApi.readText>>);
    renderDialog("/srv/notes.txt");

    expect(await screen.findByText("hello")).toBeTruthy();
  });

  it("says why a file that is too large is not shown", async () => {
    vi.mocked(sftpApi.previewFile).mockRejectedValue(new ApiError("sftp_preview_too_large", 413, null));
    renderDialog("/tmp/huge.png");

    expect(await screen.findByText(/larger than 8 MiB/u)).toBeTruthy();
    expect(sftpApi.readText).not.toHaveBeenCalled();
  });
});
