import { useRef, useState, type DragEvent } from "react";
import { canDrop, dragMimeType, noGroup, type DragPayload } from "./dragdrop";

// useConnectionDrag は、Connections の画面の2つの一覧にまたがる1回のドラッグを持つ。行とグループが
// ドラッグを始め、グループの一覧が受け取る。ブラウザは落とすまで運んでいるものを落とし先に読ませないので、
// 両方の一覧がここから読む。グループの列が無い幅でも落とせるように、ドラッグ中はグループの一覧を浮かせる。
export function useConnectionDrag({ movesDisabled, groupNames, onDrop }: {
  movesDisabled: boolean;
  groupNames: readonly string[];
  onDrop: (payload: DragPayload, target: string) => void;
}) {
  const [dragging, setDragging] = useState<DragPayload | null>(null);
  // ドラッグ中にポインタが上にある落とし先（グループの名前、グループなしは ""）。
  const [dropOver, setDropOver] = useState<string | null>(null);
  // グループの一覧を浮かせているあいだ、ドラッグを始めたポインタの横の位置。浮かせていなければ null。
  const [floatingListPointerX, setFloatingListPointerX] = useState<number | null>(null);
  const floatingListFrame = useRef(0);

  // floatGroupList が false のドラッグは、始めた場所（グループのシート）の中で落とすので、一覧を浮かせない。
  function startDrag(event: DragEvent, payload: DragPayload, { floatGroupList = true }: { floatGroupList?: boolean } = {}) {
    if (movesDisabled) return;
    event.dataTransfer.setData(dragMimeType, JSON.stringify(payload));
    event.dataTransfer.effectAllowed = "move";
    setDragging(payload);
    if (!floatGroupList) return;
    // Chromium は dragstart の中で DOM を大きく変えるとドラッグを取り消すことがあるので、次のフレームまで待つ。
    const pointerX = event.clientX;
    floatingListFrame.current = requestAnimationFrame(() => setFloatingListPointerX(pointerX));
  }

  function endDrag() {
    cancelAnimationFrame(floatingListFrame.current);
    setFloatingListPointerX(null);
    setDragging(null);
    setDropOver(null);
  }

  function accepts(target: string): boolean {
    return !movesDisabled && dragging !== null && canDrop(dragging, target, [...groupNames]);
  }

  function dropHandlers(target: string) {
    return {
      onDragOver: (event: DragEvent) => {
        if (!accepts(target)) return;
        event.preventDefault();
        event.stopPropagation();
        event.dataTransfer.dropEffect = "move";
        setDropOver(target);
      },
      onDragLeave: (event: DragEvent) => {
        // 落とし先の中のボタンへ移っただけのときも dragleave が来るので、外へ出たときだけ消す。
        if (event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)) return;
        setDropOver((current) => (current === target ? null : current));
      },
      onDrop: (event: DragEvent) => {
        if (dragging === null || !accepts(target)) return;
        event.preventDefault();
        event.stopPropagation();
        onDrop(dragging, target);
        endDrag();
      },
    };
  }

  return {
    movesDisabled,
    startDrag,
    endDrag,
    accepts,
    // 落とせるグループが1つも無いドラッグでは、一覧を浮かせても何もできないので浮かせない。
    floatingListPointerX: [...groupNames, noGroup].some(accepts) ? floatingListPointerX : null,
    isOver: (target: string) => dropOver === target,
    dropHandlers,
  };
}

export type ConnectionDrag = ReturnType<typeof useConnectionDrag>;
