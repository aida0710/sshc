import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { SFTPCompareTabsDialog } from "./SFTPCompareTabsDialog";
import { localHostAlias } from "./localHost";

const tabs = [
  { id: "local", alias: localHostAlias, path: "/home/user" },
  { id: "web", alias: "web", path: "/srv/web" },
  { id: "db", alias: "db", path: "/srv/db" },
];

it("starts with the current tab and another tab, showing the local connection name and paths", async () => {
  const onCompare = vi.fn();
  render(<SFTPCompareTabsDialog tabs={tabs} currentTabId="web" onCompare={onCompare} onDismiss={vi.fn()} />);
  const first = screen.getByRole("combobox", { name: "Tab to compare" });
  expect(first).toHaveValue("web");
  expect(screen.getByRole("combobox", { name: "Other tab" })).toHaveValue("local");
  expect(within(first).getByRole("option", { name: "Local:/home/user" })).toBeInTheDocument();
  await waitFor(() => expect(first).toHaveFocus());
  await userEvent.click(screen.getByRole("button", { name: "Compare" }));
  expect(onCompare).toHaveBeenCalledWith({ alias: "web", path: "/srv/web" }, { alias: localHostAlias, path: "/home/user" });
});

it("compares the selected pair and uses their latest paths", async () => {
  const onCompare = vi.fn();
  const props = { currentTabId: "web", onCompare, onDismiss: vi.fn() };
  const rendered = render(<SFTPCompareTabsDialog {...props} tabs={tabs} />);
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Other tab" }), "db");
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Tab to compare" }), "local");
  rendered.rerender(<SFTPCompareTabsDialog {...props} tabs={tabs.map((tab) => ({ ...tab, path: `${tab.path}/current` }))} />);
  await userEvent.click(screen.getByRole("button", { name: "Compare" }));
  expect(onCompare).toHaveBeenCalledWith({ alias: localHostAlias, path: "/home/user/current" }, { alias: "db", path: "/srv/db/current" });
});

it("prevents comparing a tab with itself, including a submitted invalid selection", () => {
  const onCompare = vi.fn();
  render(<SFTPCompareTabsDialog tabs={tabs} currentTabId="web" onCompare={onCompare} onDismiss={vi.fn()} />);
  const second = screen.getByRole("combobox", { name: "Other tab" });
  expect(within(second).getByRole("option", { name: "web:/srv/web" })).toBeDisabled();
  fireEvent.change(second, { target: { value: "web" } });
  expect(screen.getByRole("button", { name: "Compare" })).toBeDisabled();
  fireEvent.submit(screen.getByRole("button", { name: "Compare" }).closest("form")!);
  expect(onCompare).not.toHaveBeenCalled();
});

it.each([
  { tabs: [] },
  { tabs: [tabs[0]!] },
  { tabs: [tabs[0]!, { id: "empty", alias: "", path: "/" }] },
])("refuses comparison without two connected tabs", ({ tabs: available }) => {
  const onCompare = vi.fn();
  render(<SFTPCompareTabsDialog tabs={available} currentTabId="local" onCompare={onCompare} onDismiss={vi.fn()} />);
  expect(screen.getByRole("button", { name: "Compare" })).toBeDisabled();
  fireEvent.submit(screen.getByRole("button", { name: "Compare" }).closest("form")!);
  expect(onCompare).not.toHaveBeenCalled();
});

it.each(["removed", "disconnected"])("refuses a selected tab that is %s while the dialog is open", (change) => {
  const onCompare = vi.fn();
  const props = { currentTabId: "web", onCompare, onDismiss: vi.fn() };
  const rendered = render(<SFTPCompareTabsDialog {...props} tabs={tabs} />);
  const remaining = change === "removed" ? tabs.filter((tab) => tab.id !== "web") : tabs.map((tab) => tab.id === "web" ? { ...tab, alias: "" } : tab);
  rendered.rerender(<SFTPCompareTabsDialog {...props} tabs={remaining} />);
  expect(screen.getByRole("button", { name: "Compare" })).toBeDisabled();
  fireEvent.submit(screen.getByRole("button", { name: "Compare" }).closest("form")!);
  expect(onCompare).not.toHaveBeenCalled();
});

it("dismisses through Cancel and Escape without starting a comparison", async () => {
  const onCompare = vi.fn();
  const onDismiss = vi.fn();
  render(<SFTPCompareTabsDialog tabs={tabs} currentTabId="web" onCompare={onCompare} onDismiss={onDismiss} />);
  await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(onDismiss).toHaveBeenCalledTimes(1);
  await userEvent.keyboard("{Escape}");
  expect(onDismiss).toHaveBeenCalledTimes(2);
  expect(onCompare).not.toHaveBeenCalled();
});
