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

  it("shows exact vault schema versions and restores a compatible backup", async () => {
    const onOpen = vi.fn();
    const api = buildApi({
      unlockVault: vi.fn().mockRejectedValue(new ApiError("vault_schema_older", 409, {
        code: "vault_schema_older",
        message: "request rejected",
        currentVersion: 3,
        requiredVersion: 4,
      })),
      recoverCompatibleVault: vi.fn().mockResolvedValue({
        exists: true,
        unlocked: true,
        aliases: [],
        dedicatedKeyPassphrases: [],
      }),
    });
    render(
      <LanguageProvider initial="ja">
        <LockScreen exists onOpen={onOpen} api={api} />
      </LanguageProvider>,
    );

    await userEvent.type(screen.getByLabelText("マスターパスワード"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "開く" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "vault のバージョンが古いです（必要なバージョン: 4、現在: 3）。",
    );
    await userEvent.click(screen.getByRole("button", { name: "互換性のある vault を復元" }));
    await waitFor(() => expect(api.recoverCompatibleVault).toHaveBeenCalledWith("a long enough password"));
    expect(onOpen).toHaveBeenCalled();
  });

  it("requires an explicit acknowledgement before replacing an unsupported vault", async () => {
    const onOpen = vi.fn();
    const api = buildApi({
      unlockVault: vi.fn().mockRejectedValue(new ApiError("vault_schema_newer", 409, {
        code: "vault_schema_newer",
        message: "request rejected",
        currentVersion: 5,
        requiredVersion: 4,
      })),
      resetUnsupportedVault: vi.fn().mockResolvedValue({
        exists: true,
        unlocked: true,
        aliases: [],
        dedicatedKeyPassphrases: [],
      }),
    });
    render(<LockScreen exists onOpen={onOpen} api={api} />);

    await userEvent.type(screen.getByLabelText("Master password"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("supported: 4, current: 5");

    const reset = screen.getByRole("button", { name: "Create an empty vault" });
    expect(reset).toBeDisabled();
    await userEvent.click(screen.getByRole("checkbox"));
    await userEvent.click(reset);
    await waitFor(() => expect(api.resetUnsupportedVault).toHaveBeenCalledWith("a long enough password"));
    expect(onOpen).toHaveBeenCalled();
  });

  it("shows a copyable safe diagnostic when Android storage rejects vault creation", async () => {
    const api = buildApi({
      initialiseVault: vi.fn().mockRejectedValue(new ApiError("vault_storage_permission_denied", 500, {
        code: "vault_storage_permission_denied",
        message: "request rejected",
        detail: "the operating system denied access to the app's private storage",
      })),
    });
    render(<LockScreen exists={false} version="0.13.6" onOpen={vi.fn()} api={api} />);

    await userEvent.type(screen.getByLabelText("Master password"), "a long enough password");
    await userEvent.type(screen.getByLabelText("Confirm master password"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "Create the vault" }));

    expect(await screen.findByText(/Android denied access/i)).toBeInTheDocument();
    await userEvent.click(screen.getByText("Show diagnostic details"));
    expect(screen.getByText(/Version: 0\.13\.6/)).toHaveTextContent(
      "Operation: POST /api/v1/passwords/initialise",
    );
    expect(screen.getByText(/Version: 0\.13\.6/)).not.toHaveTextContent("a long enough password");
  });

  it("switches a stale creation screen to unlock when a vault already exists", async () => {
    const onExists = vi.fn();
    const api = buildApi({
      initialiseVault: vi.fn().mockRejectedValue(new ApiError("vault_already_exists", 409, null)),
      passwordVault: vi.fn().mockResolvedValue({
        exists: true,
        unlocked: false,
        aliases: [],
        dedicatedKeyPassphrases: [],
      }),
    });
    const { rerender } = render(
      <LockScreen exists={false} version="0.13.6" onOpen={vi.fn()} onExists={onExists} api={api} />,
    );

    await userEvent.type(screen.getByLabelText("Master password"), "a long enough password");
    await userEvent.type(screen.getByLabelText("Confirm master password"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "Create the vault" }));
    await waitFor(() => expect(onExists).toHaveBeenCalledTimes(1));

    rerender(<LockScreen exists version="0.13.6" onOpen={vi.fn()} onExists={onExists} api={api} />);
    expect(screen.queryByLabelText("Confirm master password")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open" })).toBeInTheDocument();
  });
});
