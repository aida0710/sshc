import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { LockScreen } from "./LockScreen";
import type { IntegrationsApi } from "../api/integrations";

function buildApi(overrides: Partial<IntegrationsApi> = {}): IntegrationsApi {
  return {
    initialiseVault: vi.fn().mockResolvedValue({ exists: true, unlocked: true, biometric: { available: false, enabled: false }, aliases: [], dedicatedKeyPassphrases: [] }),
    unlockVault: vi.fn().mockResolvedValue({ exists: true, unlocked: true, biometric: { available: false, enabled: false }, aliases: [], dedicatedKeyPassphrases: [] }),
    ...overrides,
  } as unknown as IntegrationsApi;
}

describe("LockScreen", () => {
  // vault を作ることと開くことは同じに見えて違う: 一方は
  // 取り消せない。画面はそれを、押した後ではなくフィールドの前に
  // 伝える。
  it("says a new master password cannot be recovered, and asks for it twice", async () => {
    const api = buildApi();
    const onOpen = vi.fn();
    render(<LockScreen exists={false} onOpen={onOpen} api={api} />);

    expect(screen.getByText(/cannot be recovered/i)).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText("Master password"), "a long enough password");
    // 片方のフィールドが埋まっているだけでは足りない: 誰も復元できない
    // パスワードの誤字は、最初の 1 回でユーザーを自分の vault から締め出してしまう。
    expect(screen.getByRole("button", { name: "Create the vault" })).toBeDisabled();

    await userEvent.type(screen.getByLabelText("Confirm master password"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "Create the vault" }));

    await waitFor(() => expect(api.initialiseVault).toHaveBeenCalledWith("a long enough password"));
    expect(onOpen).toHaveBeenCalled();
  });

  it("refuses a password too short to be worth deriving a key from", async () => {
    render(<LockScreen exists={false} onOpen={vi.fn()} api={buildApi()} />);

    await userEvent.type(screen.getByLabelText("Master password"), "short");
    await userEvent.type(screen.getByLabelText("Confirm master password"), "short");

    expect(screen.getByRole("button", { name: "Create the vault" })).toBeDisabled();
  });

  it("opens an existing vault with one field and no warning", async () => {
    const api = buildApi();
    const onOpen = vi.fn();
    render(<LockScreen exists onOpen={onOpen} api={api} />);

    expect(screen.queryByLabelText("Confirm master password")).not.toBeInTheDocument();
    expect(screen.queryByText(/cannot be recovered/i)).not.toBeInTheDocument();

    await userEvent.type(screen.getByLabelText("Master password"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "Open" }));

    await waitFor(() => expect(api.unlockVault).toHaveBeenCalledWith("a long enough password"));
    expect(onOpen).toHaveBeenCalled();
  });

  it("says the password was wrong rather than that something failed", async () => {
    const api = buildApi({
      unlockVault: vi.fn().mockRejectedValue(new ApiError("wrong_passphrase", 403, null)),
    });
    const onOpen = vi.fn();
    render(<LockScreen exists onOpen={onOpen} api={api} />);

    await userEvent.type(screen.getByLabelText("Master password"), "not the master password");
    await userEvent.click(screen.getByRole("button", { name: "Open" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/not the master password/i);
    // そしてシェルは閉じたままになる。
    expect(onOpen).not.toHaveBeenCalled();
  });
  // **預けてあるなら、開いた瞬間に自分から尋ねる。** 押させるための機能ではない。
  it("asks the operating system as soon as it opens, when this machine keeps something", async () => {
    const unlockWithBiometric = vi.fn().mockResolvedValue({ exists: true, unlocked: true, biometric: { available: true, enabled: true }, aliases: [], dedicatedKeyPassphrases: [] });
    const onOpen = vi.fn();
    render(
      <LockScreen exists biometric onOpen={onOpen} api={buildApi({ unlockWithBiometric })} />,
    );

    await waitFor(() => expect(unlockWithBiometric).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(onOpen).toHaveBeenCalled());
  });

  // **断られたことは失敗ではない。** パスワードの欄はそこに在るので、赤い帯を
  // 出さずに黙って戻る。
  it("says nothing when the person refuses, and leaves the password where it was", async () => {
    const unlockWithBiometric = vi.fn().mockRejectedValue(new ApiError("biometric_refused", 403, null));
    render(
      <LockScreen exists biometric onOpen={vi.fn()} api={buildApi({ unlockWithBiometric })} />,
    );

    await waitFor(() => expect(unlockWithBiometric).toHaveBeenCalled());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Master password")).toBeInTheDocument();
  });

  // 預けていない端末では、こちらから尋ねない。
  it("does not ask on a machine that keeps nothing", async () => {
    const unlockWithBiometric = vi.fn();
    render(
      <LockScreen exists onOpen={vi.fn()} api={buildApi({ unlockWithBiometric })} />,
    );

    await screen.findByLabelText("Master password");
    expect(unlockWithBiometric).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Use Touch ID" })).not.toBeInTheDocument();
  });
});
