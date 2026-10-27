import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { CredentialsApi } from "../api/credentials";
import { TOTPCurrentCode, TOTPNeighbourCodes } from "./TOTPCode";
import { useTOTPCodes } from "./useTOTPCodes";

function codeSource(): Pick<CredentialsApi, "totpCodes"> {
  return {
    totpCodes: vi.fn().mockResolvedValue({
      previous: "111111",
      current: "222222",
      next: "333333",
      periodSeconds: 30,
      remainingSeconds: 17,
    }),
  };
}

function Codes({ api, neighbours = false }: { api: Pick<CredentialsApi, "totpCodes">; neighbours?: boolean }) {
  const codes = useTOTPCodes("production-otp", api);
  return (
    <>
      <TOTPCurrentCode name="production-otp" codes={codes} />
      {neighbours && codes.state === "ready" ? <TOTPNeighbourCodes name="production-otp" codes={codes.codes} /> : null}
    </>
  );
}

describe("TOTP codes", () => {
  it("copies the code without the spacing that makes it readable", async () => {
    const user = userEvent.setup();
    render(<Codes api={codeSource()} />);

    const code = await screen.findByRole("button", { name: "Copy the current code for production-otp" });
    expect(within(code).getByText("222 222")).toBeVisible();

    await user.click(code);

    expect(await navigator.clipboard.readText()).toBe("222222");
    expect(screen.getByText("Copied.")).toBeInTheDocument();
  });

  it("shows how long the current code has left", async () => {
    render(<Codes api={codeSource()} />);

    expect(await screen.findByRole("timer", { name: "17s" })).toBeInTheDocument();
  });

  it("copies the surrounding codes", async () => {
    const user = userEvent.setup();
    render(<Codes api={codeSource()} neighbours />);

    await user.click(await screen.findByRole("button", { name: "Copy the previous code for production-otp" }));
    expect(await navigator.clipboard.readText()).toBe("111111");

    await user.click(screen.getByRole("button", { name: "Copy the next code for production-otp" }));
    expect(await navigator.clipboard.readText()).toBe("333333");
  });

  it("says the write was refused rather than claiming it succeeded", async () => {
    const user = userEvent.setup();
    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));
    render(<Codes api={codeSource()} />);

    await user.click(await screen.findByRole("button", { name: "Copy the current code for production-otp" }));

    expect(await screen.findByText(/refused to write to the clipboard/)).toBeInTheDocument();
    expect(screen.queryByText("Copied.")).not.toBeInTheDocument();
  });
});
