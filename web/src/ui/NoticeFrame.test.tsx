import { act, render, screen } from "@testing-library/react";
import { useRef, useState } from "react";
import { describe, expect, it } from "vitest";
import { NoticeFrame } from "./NoticeFrame";
import { isDismissibleLayerOpen, useDismissibleLayer } from "./useDismissibleLayer";

function Menu({ onClose }: { onClose: () => void }) {
  const container = useRef<HTMLDivElement>(null);
  useDismissibleLayer({ open: true, containerRefs: [container], onDismiss: onClose });
  return <div ref={container}><button type="button">Choose a host</button></div>;
}

// The menu closes and the card appears in one update, as when a host is chosen and its key question comes back at once.
function MenuThenCard() {
  const [chosen, setChosen] = useState(false);
  return (
    <>
      {chosen ? null : <Menu onClose={() => undefined} />}
      <button type="button" onClick={() => setChosen(true)}>choose</button>
      {chosen ? (
        <NoticeFrame tone="notice" title="Check the host key" focus={{ take: () => !isDismissibleLayerOpen(), giveBack: () => undefined }}>
          <button type="button">Connect</button>
        </NoticeFrame>
      ) : null}
    </>
  );
}

describe("NoticeFrame", () => {
  it("takes the focus when a menu closes in the same update the card appears in", () => {
    render(<MenuThenCard />);
    act(() => screen.getByRole("button", { name: "choose" }).click());
    expect(screen.getByRole("region", { name: "Check the host key" })).toHaveFocus();
  });
});
