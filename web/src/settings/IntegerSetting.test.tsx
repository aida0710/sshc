import { fireEvent, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderSettings } from "../testing/settingsDraft";
import { fakeSettingsServer } from "../testing/settingsServer";
import { FieldRow } from "../ui/FieldRow";
import { IntegerSetting } from "./IntegerSetting";

beforeEach(() => { fakeSettingsServer(); });

function renderInput() {
  const onChange = vi.fn();
  renderSettings(
    <FieldRow label="Streams" hint="How many at once" interactiveChildren>
      <IntegerSetting label="Parallel transfers" value={2} min={1} max={8} widthClass="w-20" onChange={onChange} />
    </FieldRow>,
  );
  return { onChange, input: screen.getByRole("spinbutton", { name: "Parallel transfers" }) };
}

describe("a whole number typed into the settings", () => {
  it.each(["", "0", "9", "2.5"])("keeps %j as typed, says what is accepted and hands nothing on", (typed) => {
    const { input, onChange } = renderInput();
    fireEvent.change(input, { target: { value: typed } });

    expect(input).toHaveDisplayValue(typed);
    expect(input).toBeInvalid();
    expect(screen.getByText("Parallel transfers: enter a whole number from 1 to 8.")).toBeVisible();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("is no longer wrong once it is put right", () => {
    const { input, onChange } = renderInput();
    fireEvent.change(input, { target: { value: "80" } });
    fireEvent.change(input, { target: { value: "8" } });

    expect(input).toBeValid();
    expect(screen.queryByText(/enter a whole number/)).not.toBeInTheDocument();
    expect(onChange).toHaveBeenCalledExactlyOnceWith(8);
  });
});
