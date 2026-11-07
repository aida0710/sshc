import type { TerminalHostKeyQuestion } from "../api/terminalSessions";
import { HostKeyQuestionCard } from "../shell/HostKeyQuestionCard";
import { useHostKeyAnswer } from "../shell/useHostKeyAnswer";
import { isDismissibleLayerOpen } from "../ui/useDismissibleLayer";

// SFTPHostKeyQuestion asks, where the tab's file list would be, whether to trust a host key known_hosts does not
// have, while the tab's first listing waits on the answer. The card cannot be hidden: it covers nothing (the
// connection log stays under it), and the tab has nowhere else to answer from.
export function SFTPHostKeyQuestion({ question, onAnswer }: {
  question: TerminalHostKeyQuestion;
  onAnswer: (fingerprint: string, accept: boolean) => Promise<boolean>;
}) {
  const hostKeyAnswer = useHostKeyAnswer(onAnswer);
  return (
    <HostKeyQuestionCard
      question={question}
      hostKeyAnswer={hostKeyAnswer}
      // Not from under a dialog or menu. Once answered, the stage line and then the rows take its place; they take
      // nothing back.
      focus={{ take: () => !isDismissibleLayerOpen(), giveBack: () => undefined }}
    />
  );
}
