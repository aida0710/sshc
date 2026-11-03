import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { NoticeFrame, noticeCardButton, type NoticeFocus } from "../ui/NoticeFrame";
import { Button } from "../ui/surface";

// PasswordSaveCard asks once whether to keep the password typed in the terminal for this connection. Closing the card
// is the same answer as "Don't save": it is asked only once, and a password the engine holds for this question is
// let go of as soon as the question is over.
export function PasswordSaveCard({ focus, onAnswer }: {
  focus: NoticeFocus;
  onAnswer: (save: boolean) => Promise<boolean>;
}) {
  const t = useTranslate();
  const [answering, setAnswering] = useState<boolean | null>(null);
  const [failed, setFailed] = useState(false);

  async function answer(save: boolean) {
    if (answering !== null) return;
    setAnswering(save);
    setFailed(false);
    const answered = await onAnswer(save);
    setAnswering(null);
    setFailed(!answered);
  }

  return (
    <NoticeFrame
      tone="question"
      title={t("terminal.noticeSavePassword")}
      body={failed ? t("terminal.savePasswordFailed") : undefined}
      focus={focus}
      onDismiss={() => void answer(false)}
    >
      <Button busy={answering === false} disabled={answering === true} onClick={() => void answer(false)} className={noticeCardButton}>{t("terminal.savePasswordDecline")}</Button>
      <Button kind="primary" busy={answering === true} disabled={answering === false} onClick={() => void answer(true)} className={noticeCardButton}>{t("terminal.savePasswordAccept")}</Button>
    </NoticeFrame>
  );
}
