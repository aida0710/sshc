import { act, createEvent, fireEvent, render, screen, within } from "@testing-library/react";
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
vi.mock("../editor/MonacoEditor", () => ({
  MonacoEditor: ({ path, value, readOnly }: { path: string; value: string; readOnly: boolean }) => (
    <pre {...keyboardOwnerProps} tabIndex={0} aria-label="Editor" data-path={path} data-read-only={String(readOnly)}>{value}</pre>
  ),
}));

const png = { contentType: "image/png", revision: "r1", blob: new Blob([new Uint8Array([1])]) };

function renderDialog(path: string, { paths, alias = "web", onClose = () => undefined }: { paths?: string[]; alias?: string; onClose?: () => void } = {}) {
  return render(
    <LanguageProvider>
      <FilePreviewDialog alias={alias} path={path} {...(paths === undefined ? {} : { paths })} onClose={onClose} />
    </LanguageProvider>,
  );
}

// swipe strokes one finger across the picture, from one point to another, and lets the spring that finishes the
// stroke run to its end. jsdom has no pointer capture, which a stroke takes once it moves the picture.
function swipe(area: Element, from: { x: number; y: number }, to: { x: number; y: number }, pointerId = 1) {
  Object.assign(area, { hasPointerCapture: () => false, setPointerCapture: vi.fn() });
  fireEvent.pointerDown(area, { pointerId, pointerType: "touch", clientX: from.x, clientY: from.y });
  fireEvent.pointerMove(area, { pointerId, pointerType: "touch", clientX: to.x, clientY: to.y });
  fireEvent.pointerUp(area, { pointerId, pointerType: "touch", clientX: to.x, clientY: to.y });
  act(() => {
    vi.advanceTimersByTime(1000);
  });
}

// fingerAt sends one touch pointer event at a time (milliseconds), which the speed of a let-go is read from. jsdom
// stamps an event with the real time and leaves out a timeStamp given with it, and React takes a timeStamp of 0 for
// none, so the times count from a second in.
function fingerAt(area: Element, type: "pointerDown" | "pointerMove" | "pointerUp", { x, y, time }: { x: number; y: number; time: number }) {
  const event = createEvent[type](area, { pointerId: 1, pointerType: "touch", clientX: x, clientY: y });
  Object.defineProperty(event, "timeStamp", { value: 1000 + time });
  fireEvent(area, event);
}

function zoomArea(): Element {
  const area = document.querySelector("[data-preview-zoom-area]");
  if (area === null) throw new Error("no picture is shown");
  return area;
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

  it("heads the window with the file's name and says which folder it is in", async () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    renderDialog("/srv/shots/IMG_20261103_120000_with_a_long_name.png");

    const dialog = await screen.findByRole("dialog", { name: "/srv/shots/IMG_20261103_120000_with_a_long_name.png" });
    expect(within(dialog).getByRole("heading", { level: 2 })).toHaveTextContent(/^IMG_20261103_120000_with_a_long_name\.png$/u);
    expect(within(dialog).getByText("/srv/shots")).toBeVisible();
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

  it("says a video that is not there was not found, instead of calling it a format the browser cannot play", async () => {
    vi.mocked(sftpApi.issueMediaTicket).mockRejectedValue(new ApiError("sftp_not_found", 404, null));
    renderDialog("/home/me/Pictures/IMG_2041.jpgcording.mp4");

    expect(await screen.findByText("The file was not found.")).toBeTruthy();
    expect(screen.queryByText("This browser can't play this file.")).toBeNull();
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

  it("says it is loading until the file has been read", async () => {
    let finish: (value: Awaited<ReturnType<typeof sftpApi.previewFile>>) => void = () => undefined;
    vi.mocked(sftpApi.previewFile).mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    renderDialog("/srv/slow.png");

    expect(screen.getByRole("status").textContent).toBe("Loading preview…");
    finish(png);
    await screen.findByRole("img", { name: "slow.png" });
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("says the video is loading until its first frame, and again while it waits after a seek", async () => {
    vi.mocked(sftpApi.issueMediaTicket).mockResolvedValue("ticket-3");
    vi.mocked(sftpApi.revokeMediaTicket).mockResolvedValue();
    renderDialog("/srv/long.mp4");
    await vi.waitFor(() => expect(document.querySelector("video")).not.toBeNull());
    const video = document.querySelector("video")!;

    expect(screen.getByRole("status")).toBeTruthy();
    fireEvent.loadedData(video);
    expect(screen.queryByRole("status")).toBeNull();
    fireEvent.seeking(video);
    expect(screen.getByRole("status")).toBeTruthy();
    fireEvent.seeked(video);
    expect(screen.queryByRole("status")).toBeNull();
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

  it("offers what else the shown file can do in its menu, for the file it has stepped to", async () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    const chosen = vi.fn();
    render(
      <LanguageProvider>
        <FilePreviewDialog
          alias="web"
          path="/tmp/a.png"
          paths={["/tmp/a.png", "/tmp/b.png"]}
          actions={(path) => [{ label: "Edit with SFTP", onSelect: () => chosen(path) }]}
          onClose={() => undefined}
        />
      </LanguageProvider>,
    );

    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    await userEvent.click(screen.getByRole("button", { name: "Actions for b.png" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Edit with SFTP" }));
    expect(chosen).toHaveBeenCalledWith("/tmp/b.png");
  });

  it("shows no menu when the caller offers nothing more", () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    renderDialog("/tmp/a.png");
    expect(screen.queryByRole("button", { name: "Actions for a.png" })).toBeNull();
  });

  it("shows a path that is not among the steps alone", async () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    renderDialog("/srv/plot", { paths: ["/tmp/a.png"] });
    await screen.findByRole("img", { name: "plot" });
    expect(screen.queryByRole("button", { name: "Next" })).toBeNull();
  });

  it("steps to the next and previous image with a finger's swipe, and closes on a swipe down", async () => {
    // The spring that finishes a stroke runs on animation frames; the test moves their time by hand.
    vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame", "performance"] });
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    const onClose = vi.fn();
    renderDialog("/tmp/b.png", { paths: ["/tmp/a.png", "/tmp/b.png", "/tmp/c.png"], onClose });
    await screen.findByRole("img", { name: "b.png" });

    swipe(zoomArea(), { x: 300, y: 300 }, { x: 150, y: 310 });
    expect(await screen.findByRole("img", { name: "c.png" })).toBeTruthy();
    swipe(zoomArea(), { x: 100, y: 300 }, { x: 260, y: 290 });
    expect(await screen.findByRole("img", { name: "b.png" })).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();

    swipe(zoomArea(), { x: 200, y: 100 }, { x: 210, y: 260 });
    expect(onClose).toHaveBeenCalledOnce();
    vi.useRealTimers();
  });

  it("draws the picture under the finger while it strokes, and lets a short stroke go back", async () => {
    vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame", "performance"] });
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    renderDialog("/tmp/b.png", { paths: ["/tmp/a.png", "/tmp/b.png", "/tmp/c.png"] });
    const image = await screen.findByRole("img", { name: "b.png" });
    const area = zoomArea();
    vi.spyOn(area, "getBoundingClientRect").mockReturnValue({ left: 0, top: 0, width: 400, height: 800 } as DOMRect);
    Object.assign(area, { hasPointerCapture: () => false, setPointerCapture: vi.fn() });

    fingerAt(area, "pointerDown", { x: 300, y: 300, time: 0 });
    fingerAt(area, "pointerMove", { x: 240, y: 302, time: 100 });
    expect(image.style.transform).toBe("translate(-60px, 0px) scale(1)");

    // 60px of 400, slowly: not far enough to step, so the picture goes back.
    fingerAt(area, "pointerUp", { x: 240, y: 302, time: 400 });
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.getByText("2 / 3")).toBeTruthy();
    expect(image.style.transform).toBe("translate(0px, 0px) scale(1)");
    vi.useRealTimers();
  });

  it("steps to the next file as soon as none of the picture is left in sight, before the spring settles", async () => {
    vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame", "performance"] });
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    renderDialog("/tmp/b.png", { paths: ["/tmp/a.png", "/tmp/b.png", "/tmp/c.png"] });
    const image = await screen.findByRole("img", { name: "b.png" });
    const area = zoomArea();
    vi.spyOn(area, "getBoundingClientRect").mockReturnValue({ left: 0, top: 0, width: 400, height: 800 } as DOMRect);
    Object.defineProperties(image, { offsetWidth: { value: 400 }, offsetHeight: { value: 300 }, offsetLeft: { value: 0 }, offsetTop: { value: 250 } });
    Object.assign(area, { hasPointerCapture: () => false, setPointerCapture: vi.fn() });

    fingerAt(area, "pointerDown", { x: 300, y: 300, time: 0 });
    fingerAt(area, "pointerMove", { x: 200, y: 300, time: 50 });
    fingerAt(area, "pointerUp", { x: 200, y: 300, time: 50 });
    // The spring settles past the edge in about 450ms; the picture is out of sight in about 200ms.
    act(() => {
      vi.advanceTimersByTime(250);
    });
    expect(await screen.findByText("3 / 3")).toBeTruthy();
    vi.useRealTimers();
  });

  it("gives a little where there is no file to go to, and goes back", async () => {
    vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame", "performance"] });
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    renderDialog("/tmp/a.png", { paths: ["/tmp/a.png", "/tmp/b.png"] });
    const image = await screen.findByRole("img", { name: "a.png" });
    const area = zoomArea();
    vi.spyOn(area, "getBoundingClientRect").mockReturnValue({ left: 0, top: 0, width: 400, height: 800 } as DOMRect);
    Object.assign(area, { hasPointerCapture: () => false, setPointerCapture: vi.fn() });

    fingerAt(area, "pointerDown", { x: 100, y: 300, time: 0 });
    fingerAt(area, "pointerMove", { x: 300, y: 300, time: 50 });
    const moved = Number(/translate\((-?[\d.]+)px/.exec(image.style.transform)?.[1]);
    expect(moved).toBeGreaterThan(0);
    expect(moved).toBeLessThan(200);

    fingerAt(area, "pointerUp", { x: 300, y: 300, time: 60 });
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.getByText("1 / 2")).toBeTruthy();
    expect(image.style.transform).toBe("translate(0px, 0px) scale(1)");
    vi.useRealTimers();
  });

  it("leaves a stroke across a zoomed image, and a pinch, to the zoom", async () => {
    vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame", "performance"] });
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    renderDialog("/tmp/b.png", { paths: ["/tmp/a.png", "/tmp/b.png", "/tmp/c.png"] });
    await screen.findByRole("img", { name: "b.png" });

    fireEvent.pointerDown(zoomArea(), { pointerId: 1, pointerType: "touch", clientX: 100, clientY: 300 });
    fireEvent.pointerDown(zoomArea(), { pointerId: 2, pointerType: "touch", clientX: 300, clientY: 300 });
    fireEvent.pointerUp(zoomArea(), { pointerId: 2, pointerType: "touch", clientX: 300, clientY: 300 });
    fireEvent.pointerUp(zoomArea(), { pointerId: 1, pointerType: "touch", clientX: 260, clientY: 300 });
    expect(screen.getByText("2 / 3")).toBeTruthy();

    fireEvent.doubleClick(zoomArea());
    // jsdom has no pointer capture, which a drag across the zoomed image takes.
    Object.assign(zoomArea(), { hasPointerCapture: () => false, setPointerCapture: vi.fn() });
    swipe(zoomArea(), { x: 300, y: 300 }, { x: 150, y: 300 });
    expect(screen.getByText("2 / 3")).toBeTruthy();
    expect(screen.getByRole("img", { name: "b.png" })).toBeTruthy();
    vi.useRealTimers();
  });
});
