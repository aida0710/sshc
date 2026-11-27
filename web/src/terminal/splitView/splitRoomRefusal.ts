import type { MessageKey } from "../../i18n/messages";
import { edgeDirection, type PaneEdge } from "./layout";

// splitRoomRefusalKey は、edge の側にペインを足すと、どれかのペインが最小の大きさを割るときに出す文言のキーである。
export function splitRoomRefusalKey(edge: PaneEdge): MessageKey {
  return edgeDirection(edge) === "horizontal" ? "splitView.tooNarrow" : "splitView.tooShort";
}
