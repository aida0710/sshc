import { useRef } from "react";
import { useTranslate } from "../i18n/context";
import { Button, Notice } from "./surface";
import { useQRCodeCamera } from "./useQRCodeCamera";

// QRCodeScanner is a camera viewfinder that sits in the page, not over it. accept is called with the text of
// each QR code the camera sees and returns whether it is the one wanted; other codes in view are passed over.
export function QRCodeScanner({ label, accept, onCancel }: {
  label: string;
  accept: (text: string) => boolean;
  onCancel: () => void;
}) {
  const t = useTranslate();
  const video = useRef<HTMLVideoElement>(null);
  const { problem } = useQRCodeCamera(video, accept);
  return (
    <div className="flex flex-col items-start gap-3">
      {problem === null ? (
        <video
          ref={video}
          aria-label={label}
          muted
          playsInline
          className="aspect-square w-full max-w-80 rounded-inset bg-well object-cover"
        />
      ) : (
        <Notice tone="danger">{t(problem === "denied" ? "qrScanner.cameraDenied" : "qrScanner.cameraUnavailable")}</Notice>
      )}
      <Button onClick={onCancel}>{t("qrScanner.stop")}</Button>
    </div>
  );
}
