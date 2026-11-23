import { useTranslate } from "../i18n/context";

// スマホの組み方では、名札がヘッダーやターミナルの上のバーに重なるので、アプリの上にこの高さの余白を空ける。
// 枠の太さ（3px）と名札の行の高さ（18px）を足した値にする。
export const remoteSessionTagRoom = "mobile-layout:pt-[21px]";

// RemoteSessionFrame は、リモートセッションを選んでいる間、画面全体を枠で囲み、上の端にどの sshc エンジンかの名札を出す。
// どの画面にいても、操作の相手がこのマシンではないと分かるようにする。ダイアログの上にも出し、押す場所は持たない。
// 読み上げには［リモートセッション］のボタンが同じことを伝えるので、ここは見た目だけにする。
export function RemoteSessionFrame({ alias }: { alias: string }) {
  const t = useTranslate();
  return (
    <div aria-hidden="true" data-remote-session-frame={alias} className="pointer-events-none fixed inset-0 z-above-dialog border-[3px] border-remote">
      <span className="absolute left-1/2 top-0 max-w-[70vw] -translate-x-1/2 truncate rounded-b-md bg-remote px-2.5 text-2xs font-semibold leading-[18px] text-remote-ink">
        {t("runTarget.frameLabel", { name: alias })}
      </span>
    </div>
  );
}
