import { render, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { SettingsDraftProvider } from "../settings/SettingsDraftProvider";
import { SettingsSaveBar } from "../settings/SettingsSaveBar";

// SettingsWithBar は、設定の項目を、設定の画面と同じ下書きと保存の帯の中に置く。
export function SettingsWithBar({ children }: { children: ReactNode }) {
  return <SettingsDraftProvider>{children}<SettingsSaveBar /></SettingsDraftProvider>;
}

export function renderSettings(children: ReactNode) {
  return render(<SettingsWithBar>{children}</SettingsWithBar>);
}

// saveBar は、画面に出ている保存の帯。変更が無ければ null。
export function saveBar(): HTMLElement | null {
  return document.querySelector<HTMLElement>("[data-save-bar]");
}

function saveBarButton(name: string): HTMLElement {
  const bar = saveBar();
  if (bar === null) throw new Error("保存の帯が出ていない");
  return within(bar).getByRole("button", { name });
}

export const saveButton = () => saveBarButton("Save");
// savingButton は、保存を待っている間の［保存］。「Saving…」に替わる。
export const savingButton = () => saveBarButton("Saving…");

export async function saveSettings(user: Pick<typeof userEvent, "click"> = userEvent) {
  await user.click(saveButton());
  await waitFor(() => { if (saveBar() !== null) throw new Error("変更がまだ保存されていない"); });
}

export async function discardSettings(user: Pick<typeof userEvent, "click"> = userEvent) {
  await user.click(saveBarButton("Discard changes"));
}
