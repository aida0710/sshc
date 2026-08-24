import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { LanguageProvider } from "../i18n/context";
import { LockScreen } from "./LockScreen";
import type { IntegrationsApi } from "../api/integrations";

function buildApi(overrides: Partial<IntegrationsApi> = {}): IntegrationsApi {
  return {
    initialiseVault: vi.fn().mockResolvedValue({ exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [] }),
    unlockVault: vi.fn().mockResolvedValue({ exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [] }),
    ...overrides,
  } as unknown as IntegrationsApi;
}

describe("LockScreen", () => {
  it("says a new master password cannot be recovered, and asks for it twice", async () => {
    const api = buildApi();
    const onOpen = vi.fn();
    const { container } = render(<LockScreen exists={false} onOpen={onOpen} api={api} />);

    expect(screen.getByText(/cannot be recovered/i)).toBeInTheDocument();
    expect(container.querySelector('use[href="#icon-secrets"]')).not.toBeInTheDocument();

    await userEvent.type(screen.getByLabelText("Master password"), "a long enough password");
    expect(screen.getByRole("button", { name: "Create the vault" })).toBeDisabled();

    await userEvent.type(screen.getByLabelText("Confirm master password"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "Create the vault" }));

    await waitFor(() => expect(api.initialiseVault).toHaveBeenCalledWith("a long enough password"));
    expect(onOpen).toHaveBeenCalled();
  });

  it("keeps the unlock explanation in Japanese without adding a decorative icon", () => {
    const { container } = render(
      <LanguageProvider initial="ja">
        <LockScreen exists onOpen={vi.fn()} api={buildApi()} />
      </LanguageProvider>,
    );

    expect(screen.getByText("sshc を開くにはマスターパスワードを入力してください。")).toBeInTheDocument();
    expect(container.querySelector('use[href="#icon-secrets"]')).not.toBeInTheDocument();
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
    const { container } = render(<LockScreen exists onOpen={onOpen} api={api} />);

    expect(screen.queryByLabelText("Confirm master password")).not.toBeInTheDocument();
    expect(screen.queryByText(/cannot be recovered/i)).not.toBeInTheDocument();
    expect(screen.getByText("Give your master password to open sshc.")).toBeInTheDocument();
    expect(container.querySelector('use[href="#icon-secrets"]')).not.toBeInTheDocument();

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

    expect(await screen.findByRole("alert")).toHaveTextContent(/master password is incorrect/i);
    expect(onOpen).not.toHaveBeenCalled();
  });
});
