import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QRCodeScanner } from "./QRCodeScanner";

// What the decoder "reads" from each frame, in order; the last value repeats.
let frames: (string | null)[] = [];
vi.mock("jsqr", () => ({
  default: () => {
    const text = frames.length > 1 ? frames.shift() : frames[0];
    return text === null || text === undefined ? null : { data: text };
  },
}));

function fakeCamera() {
  const stop = vi.fn();
  const getUserMedia = vi.fn().mockResolvedValue({ getTracks: () => [{ stop }] });
  vi.stubGlobal("navigator", { ...navigator, mediaDevices: { getUserMedia } });
  return { getUserMedia, stop };
}

beforeEach(() => {
  // jsdom has neither a playing <video> nor a 2D canvas; the scanner needs a frame size and pixels to hand on.
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  vi.spyOn(HTMLVideoElement.prototype, "videoWidth", "get").mockReturnValue(640);
  vi.spyOn(HTMLVideoElement.prototype, "videoHeight", "get").mockReturnValue(480);
  Object.defineProperty(HTMLMediaElement.prototype, "srcObject", { configurable: true, writable: true, value: null });
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
    drawImage: () => {},
    getImageData: () => ({ data: new Uint8ClampedArray(4) }),
  } as unknown as CanvasRenderingContext2D);
});

afterEach(() => {
  frames = [];
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("QRCodeScanner", () => {
  it("opens the rear camera without the microphone and hands each code it reads to the caller", async () => {
    const { getUserMedia } = fakeCamera();
    frames = [null, "https://example.com/", "the wanted code"];
    const accept = vi.fn((text: string) => text === "the wanted code");
    render(<QRCodeScanner label="Camera" accept={accept} onCancel={() => {}} />);

    await waitFor(() => expect(accept).toHaveBeenCalledWith("the wanted code"), { timeout: 3000 });
    expect(getUserMedia).toHaveBeenCalledWith({ audio: false, video: { facingMode: { ideal: "environment" } } });
    expect(accept).toHaveBeenCalledWith("https://example.com/");
    // The accepted code ends the scan: the same frame is not handed over again.
    const calls = accept.mock.calls.length;
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(accept).toHaveBeenCalledTimes(calls);
  });

  it("lets the camera go when the scanner is taken off the screen", async () => {
    const { getUserMedia, stop } = fakeCamera();
    frames = [null];
    const { unmount } = render(<QRCodeScanner label="Camera" accept={() => false} onCancel={() => {}} />);
    await waitFor(() => expect(getUserMedia).toHaveBeenCalled());
    await waitFor(() => expect(HTMLMediaElement.prototype.play).toHaveBeenCalled());

    unmount();

    expect(stop).toHaveBeenCalled();
  });

  it("says the camera was refused, and can still be stopped", async () => {
    vi.stubGlobal("navigator", {
      ...navigator,
      mediaDevices: { getUserMedia: vi.fn().mockRejectedValue(new DOMException("refused", "NotAllowedError")) },
    });
    const onCancel = vi.fn();
    render(<QRCodeScanner label="Camera" accept={() => false} onCancel={onCancel} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("The camera cannot be used. Allow camera access, then try again.");
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalled();
  });

  it("says the camera could not be started when the device has none", async () => {
    vi.stubGlobal("navigator", {
      ...navigator,
      mediaDevices: { getUserMedia: vi.fn().mockRejectedValue(new DOMException("none", "NotFoundError")) },
    });
    render(<QRCodeScanner label="Camera" accept={() => false} onCancel={() => {}} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("The camera could not be started.");
  });
});
