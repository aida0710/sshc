import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sftpApi, type RemoteEntry } from "./api";
import { EntryThumbnail } from "./EntryThumbnail";

const photo: RemoteEntry = { name: "photo.jpg", path: "/photos/photo.jpg", type: "file", size: 4096, mode: "0644", modifiedAt: "2026-09-01T07:00:00Z", revision: "first" };

const observed = new Set<Element>();
let report: IntersectionObserverCallback = () => undefined;

function setVisible(element: Element, visible: boolean) {
  act(() => {
    report([{ target: element, isIntersecting: visible } as IntersectionObserverEntry], {} as IntersectionObserver);
  });
}

async function waitOutTheSettleDelay() {
  await act(async () => { await vi.advanceTimersByTimeAsync(200); });
}

describe("EntryThumbnail", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    observed.clear();
    // The module keeps one observer, so this stub stays the same one for every test.
    vi.stubGlobal("IntersectionObserver", class {
      constructor(callback: IntersectionObserverCallback) { report = callback; }
      observe(element: Element) { observed.add(element); }
      unobserve(element: Element) { observed.delete(element); }
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  function box(container: HTMLElement): Element {
    const element = container.firstElementChild;
    if (element === null) throw new Error("no box");
    return element;
  }

  it("asks for nothing until the row is in view, and shows the picture in the same box", async () => {
    const thumbnail = vi.spyOn(sftpApi, "thumbnail").mockResolvedValue(new Blob(["x"], { type: "image/jpeg" }));
    const { container } = render(<EntryThumbnail alias="edge" entry={photo} boxClassName="size-6" iconClassName="size-4" />);
    await waitOutTheSettleDelay();
    expect(thumbnail).not.toHaveBeenCalled();

    setVisible(box(container), true);
    await waitOutTheSettleDelay();
    expect(thumbnail).toHaveBeenCalledOnce();
    expect(thumbnail.mock.calls[0]?.[0]).toEqual({ alias: "edge", path: "/photos/photo.jpg", revision: "first" });

    vi.useRealTimers();
    await vi.waitFor(() => expect(container.querySelector("img")).not.toBeNull());
    expect(container.querySelector("img")).toHaveAttribute("src", expect.stringMatching(/^data:image\/jpeg/u));
    expect(box(container)).toHaveClass("size-6");
  });

  it("asks for nothing when the row only passes through the view", async () => {
    const thumbnail = vi.spyOn(sftpApi, "thumbnail").mockResolvedValue(new Blob(["x"], { type: "image/jpeg" }));
    const { container } = render(<EntryThumbnail alias="edge" entry={photo} boxClassName="size-6" iconClassName="size-4" />);
    setVisible(box(container), true);
    setVisible(box(container), false);
    await waitOutTheSettleDelay();
    expect(thumbnail).not.toHaveBeenCalled();
  });

  it("stops the request when the row leaves the view", async () => {
    let signal: AbortSignal | undefined;
    vi.spyOn(sftpApi, "thumbnail").mockImplementation((_, given) => {
      signal = given;
      return new Promise(() => undefined);
    });
    const { container } = render(<EntryThumbnail alias="edge" entry={photo} boxClassName="size-6" iconClassName="size-4" />);
    setVisible(box(container), true);
    await waitOutTheSettleDelay();
    expect(signal?.aborted).toBe(false);
    setVisible(box(container), false);
    expect(signal?.aborted).toBe(true);
  });

  it("keeps the icon for a file with no picture, stays quiet while it stays in view, and asks again when it comes back", async () => {
    const thumbnail = vi.spyOn(sftpApi, "thumbnail").mockRejectedValue(new Error("sftp_preview_type"));
    const { container } = render(<EntryThumbnail alias="edge" entry={photo} boxClassName="size-6" iconClassName="size-4" />);
    setVisible(box(container), true);
    await waitOutTheSettleDelay();
    await waitOutTheSettleDelay();
    expect(thumbnail).toHaveBeenCalledOnce();
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).not.toBeNull();

    setVisible(box(container), false);
    setVisible(box(container), true);
    await waitOutTheSettleDelay();
    expect(thumbnail).toHaveBeenCalledTimes(2);
  });

  it("does not ask again for a picture it already has", async () => {
    const thumbnail = vi.spyOn(sftpApi, "thumbnail").mockResolvedValue(new Blob(["x"], { type: "image/jpeg" }));
    const { container } = render(<EntryThumbnail alias="edge" entry={photo} boxClassName="size-6" iconClassName="size-4" />);
    setVisible(box(container), true);
    await waitOutTheSettleDelay();
    vi.useRealTimers();
    await vi.waitFor(() => expect(container.querySelector("img")).not.toBeNull());
    setVisible(box(container), false);
    setVisible(box(container), true);
    await new Promise((resolve) => { setTimeout(resolve, 200); });
    expect(thumbnail).toHaveBeenCalledOnce();
  });

  it("asks for nothing while the list draws no thumbnails, or for a file that has none", async () => {
    const thumbnail = vi.spyOn(sftpApi, "thumbnail").mockResolvedValue(new Blob(["x"]));
    const { rerender } = render(<EntryThumbnail alias={null} entry={photo} boxClassName="size-6" iconClassName="size-4" />);
    rerender(<EntryThumbnail alias="edge" entry={{ ...photo, name: "notes.txt" }} boxClassName="size-6" iconClassName="size-4" />);
    await waitOutTheSettleDelay();
    expect(observed.size).toBe(0);
    expect(thumbnail).not.toHaveBeenCalled();
    expect(screen.queryByRole("img")).toBeNull();
  });
});
