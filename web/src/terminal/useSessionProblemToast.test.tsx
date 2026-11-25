import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ToastProvider } from "../ui/ToastProvider";
import { useSessionProblemToast } from "./useSessionProblemToast";

function Sessions({ problem }: { problem: string }) {
  useSessionProblemToast(problem);
  return null;
}

describe("useSessionProblemToast", () => {
  it("says the reason when an operation on a session fails, and nothing while there is none", () => {
    const { rerender } = render(<ToastProvider><Sessions problem="" /></ToastProvider>);
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
    rerender(<ToastProvider><Sessions problem="No more sessions can be opened. Close one first." /></ToastProvider>);
    expect(screen.getByRole("status")).toHaveTextContent("No more sessions can be opened. Close one first.");
  });
});
