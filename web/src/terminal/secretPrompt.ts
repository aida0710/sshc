// Whether the terminal is asking for a password is read from the screen alone, for the phone's text box to hide what is
// typed (TerminalComposer). The session list does not say so (its schema is closed, and changing it would make every
// run target update), and it could not: sudo, su and ssh on the far side ask as text, like any other program.
// A wrong answer only hides or shows a box, so this looks at the end of the text before the cursor and nothing else.

// secretPromptRowsRead is how much of a long line is read, at most once a frame. A question takes a few rows on the
// narrowest phone, and a long unbroken line pouring out of a program takes thousands.
export const secretPromptRowsRead = 16;

// The end of a question: a colon, and the asterisks some programs echo for each character typed (sudo's pwfeedback,
// and sshc's own StreamPrompter). Asterisks count so that the box stays hidden after typing some in the terminal.
const questionEnd = String.raw`\s*[:：]\s*\**\s*$`;

// What comes after the word in a question. Anything else ("Password reset email sent:") is a sentence about passwords.
const bracketed = String.raw`\s*[(（[［][^)）\]］]*[)）\]］]`;
const englishAfterWord = String.raw`(?:${bracketed})?(?:,?\s+(?:try\s+)?again)?(?:\s+for\s.*)?`;
const japaneseAfterWord = String.raw`(?:${bracketed})?(?:\s*[をの]?\s*(?:再入力|入力|確認)[^:：]*)?`;

// The word starts a word, so that "db_password:" and "~/password:" (a key or a path being edited) do not count. Whatever
// is before it may be anything: "[sudo] ", "deploy@host's ", "Retype new ", "Saved password was rejected. ".
const secretPrompts = [
  new RegExp(String.raw`(?:^|[^\p{L}\p{N}_./\\-])(?:password|pass ?phrase)${englishAfterWord}${questionEnd}`, "iu"),
  new RegExp(String.raw`(?:パスワード|パスフレーズ)${japaneseAfterWord}${questionEnd}`, "u"),
];

// asksForSecret says whether the text before the cursor ends in a question for a password or a passphrase. English and
// Japanese only, the languages of sshc's screens. A one-time code ("Verification code: ") does not count: it is good
// for a moment, and seeing it typed helps more than hiding it.
export function asksForSecret(textBeforeCursor: string): boolean {
  return secretPrompts.some((prompt) => prompt.test(textBeforeCursor));
}
