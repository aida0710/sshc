import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { sftpApi } from "./api";
import { useFilePreview, type FilePreviewState } from "./useFilePreview";

vi.mock("./api", () => ({ sftpApi: {
  previewFile: vi.fn(), readText: vi.fn(), issueMediaTicket: vi.fn(), revokeMediaTicket: vi.fn(), mediaURL: (ticket: string) => `/media/${ticket}`,
} }));

const png = { contentType: "image/png", revision: "r1", blob: new Blob([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], { type: "image/png" }) };

// renderPreview renders the hook for a path and keeps what every render returned, so that a preview shown for
// a single render is not missed.
function renderPreview(path: string | null) {
  const returned: { path: string | null; preview: FilePreviewState }[] = [];
  const hook = renderHook(({ shownPath }: { shownPath: string | null }) => {
    const preview = useFilePreview("web", shownPath);
    returned.push({ path: shownPath, preview });
    return preview;
  }, { initialProps: { shownPath: path } });
  return { ...hook, returned };
}

describe("useFilePreview", () => {
  beforeEach(() => vi.resetAllMocks());

  it("is loading from the first render of a path until the file is read", async () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    const { result, returned } = renderPreview("/tmp/first.png");

    expect(returned[0]?.preview).toEqual({ kind: "loading" });
    await waitFor(() => expect(result.current.kind).toBe("image"));
  });

  it("never returns the previous file's picture on the renders after the path moves", async () => {
    vi.mocked(sftpApi.previewFile).mockResolvedValue(png);
    const { result, rerender, returned } = renderPreview("/tmp/first.png");
    await waitFor(() => expect(result.current.kind).toBe("image"));

    let finishSecond: (value: typeof png) => void = () => undefined;
    vi.mocked(sftpApi.previewFile).mockReturnValue(new Promise((resolve) => { finishSecond = resolve; }));
    rerender({ shownPath: "/tmp/second.png" });
    const secondRenders = returned.filter(({ path }) => path === "/tmp/second.png");
    expect(secondRenders.length).toBeGreaterThan(0);
    expect(secondRenders.every(({ preview }) => preview.kind === "loading")).toBe(true);

    finishSecond(png);
    await waitFor(() => expect(result.current.kind).toBe("image"));
  });

  it("reads a file again when the path comes back to it before the other file is read", async () => {
    vi.mocked(sftpApi.issueMediaTicket).mockResolvedValueOnce("first-ticket");
    vi.mocked(sftpApi.revokeMediaTicket).mockResolvedValue(undefined);
    const { result, rerender, returned } = renderPreview("/tmp/first.mp4");
    await waitFor(() => expect(result.current).toEqual({ kind: "media", element: "video", url: "/media/first-ticket" }));

    vi.mocked(sftpApi.issueMediaTicket).mockReturnValueOnce(new Promise(() => undefined));
    rerender({ shownPath: "/tmp/second.mp4" });
    vi.mocked(sftpApi.issueMediaTicket).mockResolvedValueOnce("again-ticket");
    const before = returned.length;
    rerender({ shownPath: "/tmp/first.mp4" });

    expect(sftpApi.revokeMediaTicket).toHaveBeenCalledWith("first-ticket");
    await waitFor(() => expect(result.current).toEqual({ kind: "media", element: "video", url: "/media/again-ticket" }));
    const comingBack = returned.slice(before).map(({ preview }) => preview);
    expect(comingBack).not.toContainEqual({ kind: "media", element: "video", url: "/media/first-ticket" });
  });

  it("reads nothing for a null path", () => {
    const { result } = renderPreview(null);

    expect(result.current).toEqual({ kind: "idle" });
    expect(sftpApi.previewFile).not.toHaveBeenCalled();
  });
});
