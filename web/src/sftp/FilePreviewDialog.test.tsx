import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { LanguageProvider } from "../i18n/context";
import { keyboardOwnerProps } from "../ui/useDismissibleLayer";
import { sftpApi } from "./api";
import { FilePreviewDialog } from "./FilePreviewDialog";
import { localHostAlias } from "./localHost";
import { sftpTransferManager } from "./transferManager";

vi.mock("./api", () => ({ sftpApi: {
  previewFile: vi.fn(), readText: vi.fn(), downloadLocalFile: vi.fn(), saveDownload: vi.fn(),
  issueMediaTicket: vi.fn(), revokeMediaTicket: vi.fn(), mediaURL: (ticket: string) => `/media/${ticket}`,
} }));
vi.mock("./transferManager", () => ({ sftpTransferManager: { addDownload: vi.fn() } }));
// jsdom cannot run Monaco. The stand-in shows what the dialog asked of it and,
// like the editor, owns the keys pressed inside it.
vi.mock("./MonacoEditor", () => ({
  MonacoEditor: ({ path, value, readOnly }: { path: string; value: string; readOnly: boolean }) => (
    <pre {...keyboardOwnerProps} tabIndex={0} aria-label="Editor" data-path={path} data-read-only={String(readOnly)}>{value}</pre>
  ),
}));

const png = { contentType: "image/png", revision: "r1", blob: new Blob([new Uint8Array([1])]) };

function renderDialog(path: string, { paths, alias = "web" }: { paths?: string[]; alias?: string } = {}) {
  return render(
    <LanguageProvider>
      <FilePreviewDialog alias={alias} path={path} {...(paths === undefined ? {} : { paths })} onClose={() => undefined} />
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

  it("shows the whole of a text file in the read-only editor, named by its path", async () => {
    vi.mocked(sftpApi.previewFile).mockRejectedValue(new ApiError("sftp_preview_type", 415, null));
    const contents = `print("hello")\n${"x".repeat(70_000)}`;
    vi.mocked(sftpApi.readText).mockResolvedValue({ contents, revision: "r1" } as Awaited<ReturnType<typeof sftpApi.readText>>);
    renderDialog("/srv/plot.py");

    const editor = await screen.findByLabelText("Editor");
    expect(editor.textContent).toBe(contents);
    expect(editor.dataset.path).toBe("/srv/plot.py");
    expect(editor.dataset.readOnly).toBe("true");
  });

  it("says a file of another type cannot be previewed and offers its download", async () => {
    vi.mocked(sftpApi.previewFile).mockRejectedValue(new ApiError("sftp_preview_type", 415, null));
    vi.mocked(sftpApi.readText).mockRejectedValue(new ApiError("sftp_not_utf8", 422, null));
    vi.mocked(sftpTransferManager.addDownload).mockResolvedValue("job");
    renderDialog("/srv/app.zip");

    const reason = await screen.findByText("This file type can't be previewed.");
    const downloads = screen.getAllByRole("button", { name: "Download" });
    expect(downloads).toHaveLength(2);
    expect(reason.nextElementSibling).toBe(downloads[1]);
    await userEvent.click(downloads[1]!);
    expect(sftpTransferManager.addDownload).toHaveBeenCalledWith("web", "/srv/app.zip", "file", -1);
  });

  it("says why a file that is too large is not shown, without naming it an image", async () => {
    vi.mocked(sftpApi.previewFile).mockRejectedValue(new ApiError("sftp_preview_too_large", 413, null));
    renderDialog("/tmp/huge.png");

    expect(await screen.findByText("This file is too large to preview.")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Download" })).toHaveLength(2);
    expect(sftpApi.readText).not.toHaveBeenCalled();
  });

  it("downloads a file of the engine's own disk directly and saves it in the browser", async () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    const contents = new Blob(["bytes"]);
    vi.mocked(sftpApi.downloadLocalFile).mockResolvedValue(contents);
    vi.mocked(sftpApi.saveDownload).mockResolvedValue(null);
    renderDialog("/home/me/shot.png", { alias: localHostAlias });
    await screen.findByRole("img", { name: "shot.png" });

    await userEvent.click(screen.getByRole("button", { name: "Download" }));
    expect(sftpApi.downloadLocalFile).toHaveBeenCalledWith("/home/me/shot.png");
    expect(sftpApi.saveDownload).toHaveBeenCalledWith("/home/me/shot.png", false, [contents]);
    expect(sftpTransferManager.addDownload).not.toHaveBeenCalled();
  });

  it("says why a download could not start", async () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    vi.mocked(sftpTransferManager.addDownload).mockRejectedValue(new ApiError("sftp_transfer_limit", 429, null));
    renderDialog("/srv/shot.png");
    await screen.findByRole("img", { name: "shot.png" });

    await userEvent.click(screen.getByRole("button", { name: "Download" }));
    expect(await screen.findByRole("alert")).toBeTruthy();
  });

  it("steps to the next and previous file with the arrow keys, and stops at the ends", async () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    renderDialog("/tmp/b.png", { paths: ["/tmp/a.png", "/tmp/b.png", "/tmp/c.png"] });
    expect(await screen.findByRole("img", { name: "b.png" })).toBeTruthy();
    expect(screen.getByText("2 / 3")).toBeTruthy();

    await userEvent.keyboard("{ArrowRight}");
    expect(await screen.findByRole("img", { name: "c.png" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Next" })).toHaveProperty("disabled", true);
    await userEvent.keyboard("{ArrowRight}");
    expect(screen.getByText("3 / 3")).toBeTruthy();

    await userEvent.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(await screen.findByRole("img", { name: "a.png" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Previous" })).toHaveProperty("disabled", true);
    expect(sftpApi.previewFile).toHaveBeenLastCalledWith("web", "/tmp/a.png");
  });

  it("leaves the arrow keys pressed inside the text to the editor", async () => {
    vi.mocked(sftpApi.previewFile).mockRejectedValue(new ApiError("sftp_preview_type", 415, null));
    vi.mocked(sftpApi.readText).mockResolvedValue({ contents: "# notes", revision: "r1" } as Awaited<ReturnType<typeof sftpApi.readText>>);
    renderDialog("/srv/a.md", { paths: ["/srv/a.md", "/srv/b.md"] });

    (await screen.findByLabelText("Editor")).focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(screen.getByText("1 / 2")).toBeTruthy();
  });

  it("plays audio through a ticket without reading the file first, and revokes the ticket when it closes", async () => {
    vi.mocked(sftpApi.issueMediaTicket).mockResolvedValue("ticket-1");
    vi.mocked(sftpApi.revokeMediaTicket).mockResolvedValue();
    const { container, unmount } = renderDialog("/srv/take.mp3");

    await vi.waitFor(() => expect(container.ownerDocument.querySelector("audio")?.getAttribute("src")).toBe("/media/ticket-1"));
    expect(sftpApi.issueMediaTicket).toHaveBeenCalledWith("web", "/srv/take.mp3");
    expect(sftpApi.previewFile).not.toHaveBeenCalled();
    expect(sftpApi.readText).not.toHaveBeenCalled();
    unmount();
    expect(sftpApi.revokeMediaTicket).toHaveBeenCalledWith("ticket-1");
  });

  it("offers the download when the browser cannot play the video", async () => {
    vi.mocked(sftpApi.issueMediaTicket).mockResolvedValue("ticket-2");
    vi.mocked(sftpApi.revokeMediaTicket).mockResolvedValue();
    renderDialog("/srv/clip.mov");

    await vi.waitFor(() => expect(document.querySelector("video")).not.toBeNull());
    fireEvent.error(document.querySelector("video")!);
    expect(await screen.findByText("This browser can't play this file.")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Download" })).toHaveLength(2);
  });

  it("revokes the ticket of a file it steps away from", async () => {
    vi.mocked(sftpApi.issueMediaTicket).mockResolvedValueOnce("first").mockResolvedValueOnce("second");
    vi.mocked(sftpApi.revokeMediaTicket).mockResolvedValue();
    renderDialog("/srv/a.mp4", { paths: ["/srv/a.mp4", "/srv/b.mp4"] });
    await vi.waitFor(() => expect(document.querySelector("video")?.getAttribute("src")).toBe("/media/first"));

    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    await vi.waitFor(() => expect(document.querySelector("video")?.getAttribute("src")).toBe("/media/second"));
    expect(sftpApi.revokeMediaTicket).toHaveBeenCalledWith("first");
  });

  it("shows a path that is not among the steps alone", async () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    renderDialog("/srv/plot", { paths: ["/tmp/a.png"] });
    await screen.findByRole("img", { name: "plot" });
    expect(screen.queryByRole("button", { name: "Next" })).toBeNull();
  });
});
