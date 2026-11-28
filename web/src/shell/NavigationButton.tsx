import { forwardRef } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { MarkedProgramState } from "../terminal/programStatus";
import { ProgramStatusMark } from "../terminal/ProgramStatusMark";
import { Icon, type IconName } from "../ui/icons";
import { IconButton, type IconButtonProps } from "../ui/IconButton";

const attentionReasons: Record<MarkedProgramState, MessageKey> = {
  blocked: "shell.navigationAttentionBlocked",
  error: "shell.navigationAttentionFailed",
  done: "shell.navigationAttentionDone",
};

type NavigationButtonProps = Omit<IconButtonProps, "children"> & {
  icon: IconName;
  // iconClassName はアイコンの大きさである。置く場所の字に合わせてあるので、呼ぶ側が決める。
  iconClassName: string;
  // attention は、画面に出ていないセッションのプログラムが人を要るときの状態である（offScreenProgramStatus）。
  attention: MarkedProgramState | null;
};

// NavigationButton は、閉じているナビゲーションを開くボタンである（スマホの上のバーの ☰、PC で隠したナビゲーションの
// 列のボタン）。閉じている間は、ナビゲーションの中のセッションの行の印が見えない。そこで、画面に出ていないセッションの
// プログラムが人を要るときは、行と同じ印をアイコンの右上に重ねる。印は読み上げから隠すので、理由は名前に足す。
export const NavigationButton = forwardRef<HTMLButtonElement, NavigationButtonProps>(function NavigationButton(
  { icon, iconClassName, attention, label, ...rest },
  ref,
) {
  const t = useTranslate();
  const name = attention === null ? label : t("shell.navigationWithAttention", { label, reason: t(attentionReasons[attention]) });
  return (
    <IconButton ref={ref} label={name} {...rest}>
      <span className="relative flex">
        <Icon name={icon} className={iconClassName} />
        {/* アイコンの線に重ならないよう、角から外へずらす。 */}
        {attention === null ? null : <ProgramStatusMark state={attention} className="absolute -right-1.5 -top-2.5" />}
      </span>
    </IconButton>
  );
});
