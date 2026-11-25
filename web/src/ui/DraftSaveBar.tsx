import { useContext, useEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useTranslate } from "../i18n/context";
import { saveBarFrame, SaveBarSlotContext } from "./saveBarSlot";
import { isTextEntry } from "./softKeyboard";
import { Button } from "./surface";

type DraftSaveBarProps = {
  onDiscard: () => void;
  // onSave を渡さないときは、form の id のフォームを送信する。
  onSave?: (() => void) | undefined;
  form?: string | undefined;
  saving?: boolean;
  // saveDisabled は、ほかの操作を待つなど、入力のほかの理由でまだ保存できないとき。
  saveDisabled?: boolean;
  // inputInvalid は、足りない値や誤った値があり、直すまで保存できないとき。誤りの文は欄の下に出す。
  inputInvalid?: boolean;
  discardDisabled?: boolean;
  // note は、欄の下に出せない、保存できない理由や保存の注意。
  note?: string;
  // failure は、保存できなかった理由。note や「未保存の変更あり」の代わりに出す。
  failure?: string;
};

// typedFieldOutside は、今打っている、帯の外の欄。
function typedFieldOutside(bar: HTMLElement | null): HTMLElement | null {
  const active = document.activeElement;
  return bar !== null && isTextEntry(active) && !bar.contains(active) ? active : null;
}

// errorTextOf は、欄の誤りの文。欄が指す説明の最後に付く。Monaco の文字を受ける要素は説明を指さないので、行から探す。
function errorTextOf(field: HTMLElement): Element | null {
  const describedBy = field.getAttribute("aria-describedby")?.split(" ").at(-1);
  return describedBy === undefined
    ? field.closest("[data-field-row]")?.querySelector("[aria-live]") ?? null
    : document.getElementById(describedBy);
}

// showAboveBar は、帯の上に見えるまで送る。帯の高さは、送る側の scroll-padding が空ける（saveBarScrollRoom）。
function showAboveBar(element: Element | null): void {
  element?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
}

// DraftSaveBar は、変更があるあいだ画面の下に固定する、保存するか破棄するかを選ぶ帯（利用者が選んだ形、2026-11-25）。
// スマホでは下のバーの上に、キーボードを開いているあいだはそのすぐ上に出る。変更が無いときは置かない。
export function DraftSaveBar({
  onDiscard,
  onSave,
  form,
  saving = false,
  saveDisabled = false,
  inputInvalid = false,
  discardDisabled = false,
  note = "",
  failure = "",
}: DraftSaveBarProps) {
  const t = useTranslate();
  const bar = useRef<HTMLDivElement>(null);

  // 打ち始めに帯が出ると、画面の下のほうの欄は帯の下になる。打っている欄を、帯の上の見える所まで送る。
  useEffect(() => {
    const field = typedFieldOutside(bar.current);
    // Monaco の欄で文字を受ける要素は行の高さしかないので、欄ごと送る。
    if (field !== null) showAboveBar(field.closest(".monaco-editor") ?? field);
  }, []);

  // 誤りの文は欄の下に出るので、欄だけが見えていると帯の下になる。
  useEffect(() => {
    const field = typedFieldOutside(bar.current);
    if (inputInvalid && field !== null) showAboveBar(errorTextOf(field));
  }, [inputInvalid]);

  return (
    <SaveBarPlace>
      <div ref={bar} data-save-bar className={`${saveBarFrame} flex flex-wrap items-center justify-end gap-x-3 gap-y-2`}>
        <p role="status" className={`min-w-0 grow basis-32 text-ui ${failure === "" ? "text-ink-muted" : "text-danger"}`}>
          {failure || note || t(inputInvalid ? "draft.fixToSave" : "draft.unsaved")}
        </p>
        <div className="flex shrink-0 gap-3">
          <Button disabled={discardDisabled || saving} onClick={onDiscard}>
            {t("draft.discard")}
          </Button>
          <Button
            type={onSave === undefined ? "submit" : "button"}
            {...(form === undefined ? {} : { form })}
            kind="primary"
            busy={saving}
            busyLabel={t("busy.saving")}
            disabled={saveDisabled || inputInvalid}
            onClick={onSave}
          >
            {t("draft.save")}
          </Button>
        </div>
      </div>
    </SaveBarPlace>
  );
}

// SaveBarPlace は、帯をページか欄の下の端（SaveBarSlotContext）に置く。
function SaveBarPlace({ children }: { children: ReactNode }) {
  const slot = useContext(SaveBarSlotContext);
  return slot === null ? children : createPortal(children, slot);
}
