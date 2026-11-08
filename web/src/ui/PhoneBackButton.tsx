import { useContext } from "react";
import { Icon } from "./icons";
import { PageChromeContext } from "./pageChrome";
import { PageChromePortal } from "./PageChromePortal";

// PhoneBackButton is a page's way one step back on a phone (from an opened connection to the list), put in
// the top bar where the menu button is (see pageChrome). Without the top bar's slot (a wide screen) it shows
// nothing; the list is beside the page there.
export function PhoneBackButton({ label, onBack }: { label: string; onBack: () => void }) {
  const { phoneBackSlot } = useContext(PageChromeContext);
  if (phoneBackSlot === null) return null;
  return (
    <PageChromePortal slot={phoneBackSlot}>
      <button
        type="button"
        aria-label={label}
        title={label}
        onClick={onBack}
        className="grid h-10 w-10 shrink-0 place-items-center rounded text-ink-muted hover:bg-select-fill hover:text-ink"
      >
        <Icon name="chevronLeft" className="size-5" />
      </button>
    </PageChromePortal>
  );
}
