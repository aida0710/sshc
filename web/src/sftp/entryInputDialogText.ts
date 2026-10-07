import type { MessageKey } from "../i18n/messages";
import type { SFTPEntryActionsModel } from "./useSFTPEntryActions";

type EntryInputIntent = NonNullable<SFTPEntryActionsModel["inputIntent"]>;

type EntryInputDialogText = {
  heading: MessageKey;
  label: MessageKey;
  submit: MessageKey;
  initialValue: string;
};

// entryInputDialogText は、エントリの操作ごとに、名前やパスを尋ねるダイアログの
// 見出し・入力欄の名前・確定ボタンと、最初に入れておく値を返す。
export function entryInputDialogText(intent: EntryInputIntent, currentPath: string): EntryInputDialogText {
  switch (intent.kind) {
    case "mkdir":
      return { heading: "sftp.newFolder", label: "sftp.mkdirPrompt", submit: "sftp.newFolder", initialValue: "" };
    case "createFile":
      return { heading: "sftp.newFile", label: "sftp.newFilePrompt", submit: "sftp.newFile", initialValue: "" };
    case "rename":
      return { heading: "sftp.rename", label: "sftp.renamePrompt", submit: "sftp.rename", initialValue: intent.entry.name };
    case "duplicate":
      return {
        heading: "sftp.duplicate",
        label: "sftp.renamePrompt",
        submit: "sftp.duplicate",
        initialValue: `${intent.entry.name}.copy`,
      };
    case "moveTo":
      return { heading: "sftp.moveTo", label: "sftp.moveToPrompt", submit: "sftp.move", initialValue: currentPath };
  }
}
