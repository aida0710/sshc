import { useEffect, useRef, useState, type RefObject } from "react";

// Why the camera could not be used: the person (or the device's settings) refused it, or there is no camera
// this page can open.
export type QRCodeCameraProblem = "denied" | "unavailable";

// 1 コマを読んでから次のコマを読むまでの間隔。読み取りは 1 コマ数十ミリ秒かかるので、毎フレーム読むと
// スマホでプレビューが引っかかる。かざしてから読めるまでが遅く感じない程度に空ける。
const scanIntervalMilliseconds = 150;

// 読み取りに渡すコマの長辺の上限。カメラの解像度のまま渡すと遅く、これより粗いと画面いっぱいに
// 映した QR コードの細かいマスが潰れる。
const longestScannedSide = 960;

/** このページからカメラを開けるか。安全でないオリジン（http の LAN の IP アドレスなど）では開けない。 */
export function cameraAvailable(target: Navigator = navigator): boolean {
  return typeof target.mediaDevices?.getUserMedia === "function";
}

function scannedSize(video: HTMLVideoElement): { width: number; height: number } {
  const scale = Math.min(1, longestScannedSide / Math.max(video.videoWidth, video.videoHeight));
  return { width: Math.round(video.videoWidth * scale), height: Math.round(video.videoHeight * scale) };
}

// useQRCodeCamera shows the rear camera in the video element and reads QR codes from it until accept
// returns true for one. The camera is let go when the caller unmounts.
export function useQRCodeCamera(
  video: RefObject<HTMLVideoElement | null>,
  accept: (text: string) => boolean,
): { problem: QRCodeCameraProblem | null } {
  const [problem, setProblem] = useState<QRCodeCameraProblem | null>(null);
  // The newest accept is read at each frame, so a caller's inline function does not restart the camera.
  const latestAccept = useRef(accept);
  latestAccept.current = accept;

  useEffect(() => {
    let stopped = false;
    let stream: MediaStream | null = null;
    let nextScan = 0;
    const frame = document.createElement("canvas");

    async function start() {
      const element = video.current;
      if (element === null) return;
      try {
        const [{ default: decodeQRCode }, opened] = await Promise.all([
          import("jsqr"),
          navigator.mediaDevices.getUserMedia({ audio: false, video: { facingMode: { ideal: "environment" } } }),
        ]);
        stream = opened;
        if (stopped) {
          release();
          return;
        }
        element.srcObject = opened;
        await element.play();
        const context = frame.getContext("2d", { willReadFrequently: true });
        if (context === null) throw new Error("no 2d canvas");
        const scan = () => {
          if (stopped) return;
          if (element.videoWidth > 0 && element.videoHeight > 0) {
            const { width, height } = scannedSize(element);
            frame.width = width;
            frame.height = height;
            context.drawImage(element, 0, 0, width, height);
            const read = decodeQRCode(context.getImageData(0, 0, width, height).data, width, height, {
              inversionAttempts: "dontInvert",
            });
            if (read !== null && latestAccept.current(read.data)) return;
          }
          nextScan = window.setTimeout(scan, scanIntervalMilliseconds);
        };
        scan();
      } catch (caught) {
        if (stopped) return;
        const name = caught instanceof DOMException ? caught.name : "";
        setProblem(name === "NotAllowedError" || name === "SecurityError" ? "denied" : "unavailable");
      }
    }

    function release() {
      stream?.getTracks().forEach((track) => track.stop());
      stream = null;
    }

    void start();
    return () => {
      stopped = true;
      window.clearTimeout(nextScan);
      release();
    };
  }, [video]);

  return { problem };
}
