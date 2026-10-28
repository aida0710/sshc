import { useState, type KeyboardEvent } from "react";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { Button } from "../ui/surface";
import { keyboardOwnerProps } from "../ui/useDismissibleLayer";
import { groupSegmentProblem } from "./groupNameRules";

type NewGroupFieldProps = {
  groupNames: readonly string[];
  // onCreate は、グループを作れたかを返す。作れたら onCreated で、そのグループを選ばせる。
  onCreate: (name: string) => Promise<boolean>;
  onCreated: (name: string) => void;
};

// NewGroupField は、接続先を作る欄の［新しいグループ］である。押すとその場に名前の欄を開き、
// いちばん上の階層にグループを作る。ほかのダイアログを重ねずに、作りかけの接続先を残したまま作れる。
export function NewGroupField({ groupNames, onCreate, onCreated }: NewGroupFieldProps) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");

  function close() {
    setOpen(false);
    setName("");
    setProblem("");
  }

  async function create() {
    const taken = new Set(groupNames.map((group) => group.toLowerCase()));
    const found = groupSegmentProblem(t, { parent: "", value: name, current: null, taken });
    if (found !== "") {
      setProblem(found);
      return;
    }
    setBusy(true);
    const created = await onCreate(name);
    setBusy(false);
    if (!created) {
      setProblem(t("conn.createGroupFailed"));
      return;
    }
    onCreated(name);
    close();
  }

  // この欄は接続先を作るフォームの中にあるので、Enter でフォームを送らず、グループを作る。
  // Esc は欄だけを閉じ、作りかけの接続先の画面は閉じない（欄がキーを先に受け取る keyboardOwnerProps）。
  function keyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Enter" && event.target instanceof HTMLInputElement) {
      event.preventDefault();
      if (name !== "" && !busy) void create();
    } else if (event.key === "Escape") {
      event.preventDefault();
      close();
    }
  }

  if (!open) {
    return (
      <Button className="mt-2 inline-flex items-center gap-1.5" onClick={() => setOpen(true)}>
        <Icon name="plus" className="size-4" />
        {t("tree.newGroup")}
      </Button>
    );
  }
  return (
    <div className="mt-2 flex flex-col gap-1.5">
      <div {...keyboardOwnerProps} onKeyDown={keyDown} className="flex items-center gap-2">
        <input
          autoFocus
          aria-label={t("conn.createNewGroupName")}
          value={name}
          placeholder="work"
          disabled={busy}
          onChange={(event) => {
            setName(event.currentTarget.value);
            setProblem("");
          }}
          aria-invalid={problem !== ""}
          className={`${control} min-w-0 flex-1`}
        />
        <Button busy={busy} disabled={name === ""} onClick={() => void create()}>{t("groupDialog.create")}</Button>
        <Button disabled={busy} onClick={close}>{t("groupDialog.cancel")}</Button>
      </div>
      {problem === "" ? null : <p role="alert" className="text-xs text-danger">{problem}</p>}
    </div>
  );
}
