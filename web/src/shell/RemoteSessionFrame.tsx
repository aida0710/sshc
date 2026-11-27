import type { CSSProperties } from "react";
import { createPortal } from "react-dom";
import { type CornerReach, type ScreenCorners, useScreenCorners } from "../android/screenCorners";
import { useTranslate } from "../i18n/context";
import { readableInkOn } from "../theme/accentColor";

// スマホの組み方では、名札がヘッダーやターミナルの上のバーに重なるので、アプリの上にこの高さの余白を空ける。
// 高さは index.css の --remote-session-tag-room で、ダイアログも同じだけ下げる。
export const remoteSessionTagRoom = "mobile-layout:pt-(--remote-session-tag-room)";

// RemoteSessionFrame は、リモートセッションを選んでいる間、画面全体を枠で囲み、上の端にどの sshc エンジンかの名札を出す。
// どの画面にいても、操作の相手がこのマシンではないと分かるようにする。ダイアログの上にも出し、押す場所は持たない。
// 読み上げには［リモートセッション］のボタンが同じことを伝えるので、ここは見た目だけにする。
// body の直下に出す。スマホの組み方ではアプリ全体が position: fixed で重なりの基準になり、その中に置くと、
// body の直下に出るダイアログ（ModalShell）の下に隠れるため。
// color は枠と名札の色（remoteSessionFrameColor）。null はさし色（テーマの --ui-remote）。名札の文字は、その色の上で読める黒か白にする。
// Android のアプリでは、枠の四隅を画面の角の丸みに合わせて丸め、角で枠が切れないようにする。
export function RemoteSessionFrame({ alias, color }: { alias: string; color: string | null }) {
  const t = useTranslate();
  const corners = useScreenCorners();
  const colors = color === null ? {} : { "--ui-remote": color, "--ui-remote-ink": readableInkOn(color) } as CSSProperties;
  return createPortal(
    <div aria-hidden="true" data-remote-session-frame={alias} style={{ ...colors, ...roundedToScreen(corners) }} className="pointer-events-none fixed inset-0 z-above-dialog border-[3px] border-remote">
      <span className="absolute left-1/2 top-0 max-w-[70vw] -translate-x-1/2 truncate rounded-b-md bg-remote px-2.5 text-2xs font-semibold leading-[18px] text-remote-ink">
        {t("runTarget.frameLabel", { name: alias })}
      </span>
    </div>,
    document.body,
  );
}

function cornerRadius([horizontal, vertical]: CornerReach): string {
  return `${horizontal}px ${vertical}px`;
}

// roundedToScreen は、枠の四隅の丸め（border-radius）を、画面の角の丸みの入り込みに合わせる。ブラウザでは丸めない。
function roundedToScreen(corners: ScreenCorners | null): CSSProperties {
  if (corners === null) return {};
  return {
    borderTopLeftRadius: cornerRadius(corners.topLeft),
    borderTopRightRadius: cornerRadius(corners.topRight),
    borderBottomRightRadius: cornerRadius(corners.bottomRight),
    borderBottomLeftRadius: cornerRadius(corners.bottomLeft),
  };
}
