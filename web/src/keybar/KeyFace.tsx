import type { Translate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { keyLook } from "./keyBarCatalog";
import type { KeyBarKey } from "./keySequences";

// KeyFace is what a key of the key bar shows, the same on the bar and on the settings page.
export function KeyFace({ barKey, iconClassName }: { barKey: KeyBarKey; iconClassName: string }) {
  const { face } = keyLook(barKey);
  return face.kind === "icon" ? <Icon name={face.icon} className={`mx-auto ${iconClassName}`} /> : <>{face.text}</>;
}

// spokenKeyName is the name a screen reader gives a key whose face does not say it well, or undefined to read
// the face.
export function spokenKeyName(t: Translate, barKey: KeyBarKey): string | undefined {
  const { spoken } = keyLook(barKey);
  if (spoken === undefined) return undefined;
  return "message" in spoken ? t(spoken.message) : spoken.text;
}
