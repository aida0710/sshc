import { useTranslate } from "../i18n/context";

// RemoteSessionFrame は、リモートセッションを選んでいる間、画面全体を枠で囲み、上の端にどの sshc エンジンかの名札を出す。
// どの画面にいても、操作の相手がこのマシンではないと分かるようにする。ダイアログの上にも出し、押す場所は持たない。
// 読み上げには［リモートセッション］のボタンが同じことを伝えるので、ここは見た目だけにする。
// スマホの組み方では、ヘッダーのボタンに重ならないよう、名札を接続名だけにする。
export function RemoteSessionFrame({ alias }: { alias: string }) {
  const t = useTranslate();
  return (
    <div aria-hidden="true" data-remote-session-frame={alias} className="pointer-events-none fixed inset-0 z-above-dialog border-[3px] border-remote">
      <span className="absolute left-1/2 top-0 max-w-[70vw] -translate-x-1/2 truncate rounded-b-md bg-remote px-2.5 text-2xs font-semibold leading-[18px] text-remote-ink mobile-layout:max-w-[40vw]">
        <span className="mobile-layout:hidden">{t("runTarget.frameLabel", { name: alias })}</span>
        <span className="desktop-layout:hidden">{alias}</span>
      </span>
    </div>
  );
}
