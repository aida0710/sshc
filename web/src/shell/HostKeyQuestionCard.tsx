import type { TerminalHostKeyQuestion } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";
import { NoticeFrame, noticeCardButton, type NoticeFocus } from "../ui/NoticeFrame";
import { hostKeyTypeName } from "./hostKeyTypeName";
import type { HostKeyAnswer } from "./useHostKeyAnswer";

// HostKeyQuestionCard asks whether to trust a host key known_hosts does not have, as the terminal does in words: the
// person compares the fingerprint with the one the host's administrator gave. The terminal shows it over its last
// lines, and an SFTP tab where its file list would be, both while the connection waits on the answer.
export function HostKeyQuestionCard({ question, hostKeyAnswer, focus, onDismiss }: {
  question: TerminalHostKeyQuestion;
  hostKeyAnswer: HostKeyAnswer;
  focus: NoticeFocus;
  onDismiss?: (() => void) | undefined;
}) {
  const t = useTranslate();
  const { answer, busy } = hostKeyAnswer;
  return (
    <NoticeFrame
      tone="question"
      kicker={t("terminal.noticeFirstConnection")}
      title={t("terminal.noticeCheckHostKey", { target: question.alias || question.hostName })}
      detail={<p className="mt-1.5 break-all font-mono text-xs leading-5 text-ink-muted">{hostKeyTypeName(question.keyType)} {question.fingerprint}</p>}
      focus={focus}
      onDismiss={onDismiss}
    >
      {answer === undefined ? null : (
        <>
          <Button disabled={busy} onClick={() => answer(question.fingerprint, false)} className={noticeCardButton}>{t("terminal.hostKeyReject")}</Button>
          <Button kind="primary" busy={busy} onClick={() => answer(question.fingerprint, true)} className={noticeCardButton}>{t("terminal.hostKeyAccept")}</Button>
        </>
      )}
    </NoticeFrame>
  );
}
