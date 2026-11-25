import type { MessageKey } from "../i18n/messages";
import type { SFTPEntryActionsModel } from "./useSFTPEntryActions";

type EntryInputIntent = NonNullable<SFTPEntryActionsModel["inputIntent"]>;

type EntryInputDialogText = {
  heading: MessageKey;
  label: MessageKey;
  submit: MessageKey;
  // busy は、確定して待っている間の確定ボタンの文言。
  busy: MessageKey;
  initialValue: string;
};

// entryInputDialogText は、エントリの操作ごとに、名前やパスを尋ねるダイアログの
// 見出し・入力欄の名前・確定ボタンとその待つ間の文言と、最初に入れておく値を返す。
export function entryInputDialogText(intent: EntryInputIntent, currentPath: string): EntryInputDialogText {
  switch (intent.kind) {
    case "mkdir":
      return { heading: "sftp.newFolder", label: "sftp.mkdirPrompt", submit: "sftp.newFolder", busy: "busy.creating", initialValue: "" };
    case "createFile":
      return { heading: "sftp.newFile", label: "sftp.newFilePrompt", submit: "sftp.newFile", busy: "busy.creating", initialValue: "" };
    case "rename":
      return { heading: "sftp.rename", label: "sftp.renamePrompt", submit: "sftp.rename", busy: "busy.renaming", initialValue: intent.entry.name };
    case "duplicate":
      return {
        heading: "sftp.duplicate",
        label: "sftp.renamePrompt",
        submit: "sftp.duplicate",
        busy: "busy.copying",
        initialValue: `${intent.entry.name}.copy`,
      };
    case "moveTo":
      return { heading: "sftp.moveTo", label: "sftp.moveToPrompt", submit: "sftp.move", busy: "busy.moving", initialValue: currentPath };
  }
}
