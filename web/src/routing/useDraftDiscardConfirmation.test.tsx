import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { expect, it, vi } from "vitest";
import { captureNavigationBlocker } from "../testing/navigationBlocker";
import { DiscardDraftDialog } from "../ui/DiscardDraftDialog";
import type { BrowserLocation, NavigationBlocker } from "./useSectionRoute";
import { useDraftDiscardConfirmation } from "./useDraftDiscardConfirmation";

// DraftScreen は、下書きを1つ持つ画面である。空でない下書きを未保存とみなす。
function DraftScreen({
  onNavigationBlockerChange,
  onNavigateLocation,
  navigationKeepsDraft,
}: {
  onNavigationBlockerChange: (blocker: NavigationBlocker | null) => void;
  onNavigateLocation: (url: string) => void;
  navigationKeepsDraft?: (next: BrowserLocation) => boolean;
}) {
  const [draft, setDraft] = useState("");
  const confirmation = useDraftDiscardConfirmation({
    dirty: draft !== "",
    discard: () => setDraft(""),
    navigationKeepsDraft,
    onNavigationBlockerChange,
    onNavigateLocation,
  });
  return (
    <>
      <input aria-label="Draft" value={draft} onChange={(event) => setDraft(event.target.value)} />
      {confirmation.confirming ? (
        <DiscardDraftDialog id="draft-discard" onConfirm={confirmation.confirmDiscard} onCancel={confirmation.keepEditing} />
      ) : null}
    </>
  );
}

function renderDraftScreen(navigationKeepsDraft?: (next: BrowserLocation) => boolean) {
  const navigation = captureNavigationBlocker();
  const onNavigateLocation = vi.fn();
  render(
    <DraftScreen
      onNavigationBlockerChange={navigation.onNavigationBlockerChange}
      onNavigateLocation={onNavigateLocation}
      {...(navigationKeepsDraft === undefined ? {} : { navigationKeepsDraft })}
    />,
  );
  return { navigation, onNavigateLocation };
}

it("stops leaving while the draft is unsaved, and leaves once the discard is confirmed", async () => {
  const user = userEvent.setup();
  const { navigation, onNavigateLocation } = renderDraftScreen();
  expect(navigation.registered()).toBe(false);

  await user.type(screen.getByLabelText("Draft"), "Host lab");

  expect(navigation.tryNavigate("/keys", "?key=a")).toBe(false);
  expect(screen.getByRole("dialog", { name: "Discard unsaved changes?" })).toBeVisible();
  expect(onNavigateLocation).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Discard" }));

  expect(onNavigateLocation).toHaveBeenCalledWith("/keys?key=a");
  expect(screen.getByLabelText("Draft")).toHaveValue("");
  expect(navigation.registered()).toBe(false);
});

it("keeps the draft and stays when editing is continued", async () => {
  const user = userEvent.setup();
  const { navigation, onNavigateLocation } = renderDraftScreen();
  await user.type(screen.getByLabelText("Draft"), "Host lab");

  navigation.tryNavigate("/keys");
  await user.click(screen.getByRole("button", { name: "Keep editing" }));

  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.getByLabelText("Draft")).toHaveValue("Host lab");
  expect(onNavigateLocation).not.toHaveBeenCalled();
  expect(navigation.registered()).toBe(true);
});

it("lets a navigation through without asking when the screen keeps the draft there", async () => {
  const user = userEvent.setup();
  const staysOnConfig = (next: BrowserLocation) => next.pathname === "/config";
  const { navigation } = renderDraftScreen(staysOnConfig);
  await user.type(screen.getByLabelText("Draft"), "Host lab");

  expect(navigation.tryNavigate("/config")).toBe(true);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.getByLabelText("Draft")).toHaveValue("Host lab");
});

it("has the browser ask before a reload only while the draft is unsaved", async () => {
  const user = userEvent.setup();
  renderDraftScreen();
  const reload = () => {
    const event = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(event);
    return event.defaultPrevented;
  };

  expect(reload()).toBe(false);
  await user.type(screen.getByLabelText("Draft"), "Host lab");
  expect(reload()).toBe(true);
  await user.clear(screen.getByLabelText("Draft"));
  expect(reload()).toBe(false);
});
