import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { SnippetVariableInputs } from "./SnippetVariableInputs";

describe("SnippetVariableInputs", () => {
  it("names a field by the variable, without the snippet's {{ }} around it", () => {
    render(
      <LanguageProvider initial="ja">
        <SnippetVariableInputs variables={[{ name: "lines", type: "integer", required: true, description: "表示する行数" }]} inputs={{}} onChange={vi.fn()} />
      </LanguageProvider>,
    );

    expect(screen.getByText("lines")).toBeVisible();
    expect(screen.getByText(/表示する行数/u)).toBeVisible();
    expect(screen.queryByText("{{lines}}")).toBeNull();
  });

  it("says which default an empty field falls back to, and hands each change with its variable", () => {
    const onChange = vi.fn();
    render(
      <LanguageProvider initial="ja">
        <SnippetVariableInputs
          variables={[
            { name: "count", type: "integer", required: false, default: "3" },
            { name: "verbose", type: "boolean", required: false, default: "false" },
          ]}
          inputs={{}}
          onChange={onChange}
        />
      </LanguageProvider>,
    );

    expect(screen.getByRole("spinbutton", { name: "count" })).toHaveAttribute("placeholder", "既定値を使用：3");
    expect(screen.getByRole("option", { name: "既定値を使用（false）" })).toBeInTheDocument();

    fireEvent.change(screen.getByRole("combobox", { name: "verbose" }), { target: { value: "true" } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ name: "verbose" }), "true");
  });
});
