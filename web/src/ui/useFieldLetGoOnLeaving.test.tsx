import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useFieldLetGoOnLeaving } from "./useFieldLetGoOnLeaving";
import { matchesMediaQuery } from "./useMediaQuery";

vi.mock("./useMediaQuery", async () => {
  const actual = await vi.importActual<typeof import("./useMediaQuery")>("./useMediaQuery");
  return { ...actual, matchesMediaQuery: vi.fn() };
});

function Field() {
  useFieldLetGoOnLeaving();
  return <textarea aria-label="送る文" />;
}

function setPageHidden(hidden: boolean) {
  vi.spyOn(document, "hidden", "get").mockReturnValue(hidden);
  document.dispatchEvent(new Event("visibilitychange"));
}

describe("useFieldLetGoOnLeaving", () => {
  beforeEach(() => {
    vi.mocked(matchesMediaQuery).mockReturnValue(true);
    document.documentElement.dataset.keyboardOpen = "false";
  });

  afterEach(() => {
    vi.restoreAllMocks();
    delete document.documentElement.dataset.keyboardOpen;
  });

  it("lets go of the field when a phone leaves with the keyboard closed", () => {
    render(<Field />);
    screen.getByLabelText("送る文").focus();
    setPageHidden(true);
    expect(screen.getByLabelText("送る文")).not.toHaveFocus();
  });

  it("keeps the field when the person left in the middle of typing", () => {
    render(<Field />);
    screen.getByLabelText("送る文").focus();
    document.documentElement.dataset.keyboardOpen = "true";
    setPageHidden(true);
    expect(screen.getByLabelText("送る文")).toHaveFocus();
  });

  it("commits what the keyboard is still composing and keeps the field while typing", () => {
    render(<Field />);
    const field = screen.getByLabelText("送る文");
    field.focus();
    document.documentElement.dataset.keyboardOpen = "true";
    const committed = vi.fn();
    field.addEventListener("blur", committed);
    fireEvent.compositionStart(field);
    setPageHidden(true);
    expect(committed).toHaveBeenCalledTimes(1);
    expect(field).toHaveFocus();
  });

  it("leaves a field alone once the composition has ended", () => {
    render(<Field />);
    const field = screen.getByLabelText("送る文");
    field.focus();
    document.documentElement.dataset.keyboardOpen = "true";
    const committed = vi.fn();
    field.addEventListener("blur", committed);
    fireEvent.compositionStart(field);
    fireEvent.compositionEnd(field);
    setPageHidden(true);
    expect(committed).not.toHaveBeenCalled();
  });

  it("keeps the field on a screen without an on-screen keyboard", () => {
    vi.mocked(matchesMediaQuery).mockReturnValue(false);
    render(<Field />);
    screen.getByLabelText("送る文").focus();
    setPageHidden(true);
    expect(screen.getByLabelText("送る文")).toHaveFocus();
  });

  it("keeps the field when the page comes back", () => {
    render(<Field />);
    screen.getByLabelText("送る文").focus();
    setPageHidden(false);
    expect(screen.getByLabelText("送る文")).toHaveFocus();
  });
});
