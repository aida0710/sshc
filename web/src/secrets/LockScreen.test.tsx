import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { vaultStatus } from "../testing/vaultStatus";
import { ApiError } from "../api/client";
import { LanguageProvider } from "../i18n/context";
import { ThemeProvider } from "../theme/context";
import { LockScreen } from "./LockScreen";
import type { VaultApi } from "../api/vault";

function buildApi(overrides: Partial<VaultApi> = {}): VaultApi {
  return {
    initialiseVault: vi.fn().mockResolvedValue(vaultStatus()),
    unlockVault: vi.fn().mockResolvedValue(vaultStatus()),
    ...overrides,
  } as unknown as VaultApi;
}

describe("LockScreen", () => {
  it("offers appearance and language controls before creating or opening the vault", async () => {
    render(
      <ThemeProvider initial="dark">
        <LanguageProvider initial="en">
          <LockScreen exists={false} minPassphraseLength={4} onOpen={vi.fn()} api={buildApi()} />
        </LanguageProvider>
      </ThemeProvider>,
    );

    await userEvent.selectOptions(screen.getByLabelText("Theme menu"), "light");
    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-theme", "light"));
    await userEvent.selectOptions(screen.getByLabelText("Lang menu"), "ja");
    expect(screen.getByLabelText("テーマメニュー")).toHaveValue("light");
    expect(screen.getByRole("button", { name: "Vaultを作成" })).toBeInTheDocument();
  });

  it("says a new master password cannot be recovered, and asks for it twice", async () => {
    const api = buildApi();
    const onOpen = vi.fn();
    const { container } = render(<LockScreen exists={false} minPassphraseLength={4} onOpen={onOpen} api={api} />);

    expect(screen.getByText(/cannot be recovered/i)).toBeInTheDocument();
    expect(container.querySelector('[data-icon="secrets"]')).not.toBeInTheDocument();

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

    expect(screen.getByText("sshcを開くにはマスターパスワードを入力してください。")).toBeInTheDocument();
    expect(container.querySelector('[data-icon="secrets"]')).not.toBeInTheDocument();
  });

  it("refuses a password too short to be worth deriving a key from", async () => {
    render(<LockScreen exists={false} minPassphraseLength={4} onOpen={vi.fn()} api={buildApi()} />);

    await userEvent.type(screen.getByLabelText("Master password"), "abc");
    await userEvent.type(screen.getByLabelText("Confirm master password"), "abc");

    expect(screen.getByRole("button", { name: "Create the vault" })).toBeDisabled();
  });

  it("requires the minimum length the engine reports when creating a vault", async () => {
    render(<LockScreen exists={false} minPassphraseLength={6} onOpen={vi.fn()} api={buildApi()} />);

    await userEvent.type(screen.getByLabelText("Master password"), "abcde");
    await userEvent.type(screen.getByLabelText("Confirm master password"), "abcde");
    expect(screen.getByRole("button", { name: "Create the vault" })).toBeDisabled();

    await userEvent.type(screen.getByLabelText("Master password"), "f");
    await userEvent.type(screen.getByLabelText("Confirm master password"), "f");
    expect(screen.getByRole("button", { name: "Create the vault" })).toBeEnabled();
  });

  it("does not create a password vault before the engine has reported its minimum", async () => {
    render(<LockScreen exists={false} onOpen={vi.fn()} api={buildApi()} />);

    await userEvent.type(screen.getByLabelText("Master password"), "a long enough password");
    await userEvent.type(screen.getByLabelText("Confirm master password"), "a long enough password");
    expect(screen.getByRole("button", { name: "Create the vault" })).toBeDisabled();
  });

  it("opens an existing vault with one field and no warning", async () => {
    const api = buildApi();
    const onOpen = vi.fn();
    const { container } = render(<LockScreen exists onOpen={onOpen} api={api} />);

    expect(screen.queryByLabelText("Confirm master password")).not.toBeInTheDocument();
    expect(screen.queryByText(/cannot be recovered/i)).not.toBeInTheDocument();
    expect(screen.getByText("Enter your master password to unlock sshc.")).toBeInTheDocument();
    expect(container.querySelector('[data-icon="secrets"]')).not.toBeInTheDocument();

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

  it("says unlocking failed when the engine refuses an unlock for an unknown reason", async () => {
    const api = buildApi({ unlockVault: vi.fn().mockRejectedValue(new ApiError("internal_error", 500, null)) });
    render(
      <LanguageProvider initial="ja">
        <LockScreen exists onOpen={vi.fn()} api={api} />
      </LanguageProvider>,
    );

    await userEvent.type(screen.getByLabelText("マスターパスワード"), "the master password");
    await userEvent.click(screen.getByRole("button", { name: "ロックを解除" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("ロックを解除できませんでした。");
  });

  it("says creating failed when the engine refuses a new vault for an unknown reason", async () => {
    const api = buildApi({ initialiseVault: vi.fn().mockRejectedValue(new ApiError("internal_error", 500, null)) });
    render(
      <LanguageProvider initial="ja">
        <LockScreen exists={false} minPassphraseLength={4} onOpen={vi.fn()} api={api} />
      </LanguageProvider>,
    );

    await userEvent.type(screen.getByLabelText("マスターパスワード"), "a long enough password");
    await userEvent.type(screen.getByLabelText("マスターパスワード（確認）"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "Vaultを作成" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Vaultを作成できませんでした。");
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
      recoverCompatibleVault: vi.fn().mockResolvedValue(vaultStatus()),
    });
    render(
      <LanguageProvider initial="ja">
        <LockScreen exists onOpen={onOpen} api={api} />
      </LanguageProvider>,
    );

    await userEvent.type(screen.getByLabelText("マスターパスワード"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "ロックを解除" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Vaultのバージョンが古いです（必要なバージョン：4、現在：3）。",
    );
    await userEvent.click(screen.getByRole("button", { name: "互換性のあるVaultを復元" }));
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
      resetUnsupportedVault: vi.fn().mockResolvedValue(vaultStatus()),
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

  it("says how to reduce the local backups when restoring or replacing needs to re-encrypt too many", async () => {
    const tooMany = new ApiError("vault_backups_too_many", 409, { code: "vault_backups_too_many", message: "request rejected" });
    const api = buildApi({
      unlockVault: vi.fn().mockRejectedValue(new ApiError("vault_schema_newer", 409, {
        code: "vault_schema_newer",
        message: "request rejected",
        currentVersion: 5,
        requiredVersion: 4,
      })),
      recoverCompatibleVault: vi.fn().mockRejectedValue(tooMany),
      resetUnsupportedVault: vi.fn().mockRejectedValue(tooMany),
    });
    render(
      <LanguageProvider initial="ja">
        <LockScreen exists onOpen={vi.fn()} api={api} />
      </LanguageProvider>,
    );

    await userEvent.type(screen.getByLabelText("マスターパスワード"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "ロックを解除" }));
    await userEvent.click(await screen.findByRole("button", { name: "互換性のあるVaultを復元" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("~/.ssh/sshc/backupsから古いフォルダを削除してから");

    await userEvent.click(screen.getByRole("checkbox"));
    await userEvent.click(screen.getByRole("button", { name: "空のVaultを作成" }));
    await waitFor(() => expect(api.resetUnsupportedVault).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("ローカルのバックアップが多すぎるため"));
    expect(screen.getByRole("alert")).not.toHaveTextContent("安全に置き換えられませんでした");
  });

  it("explains a failed migration and says that the original vault remains", async () => {
    const api = buildApi({
      unlockVault: vi.fn().mockRejectedValue(new ApiError("vault_migration_failed", 409, {
        code: "vault_migration_failed",
        message: "request rejected",
        currentVersion: 4,
        requiredVersion: 5,
      })),
    });
    render(
      <LanguageProvider initial="ja">
        <LockScreen exists onOpen={vi.fn()} api={api} />
      </LanguageProvider>,
    );

    await userEvent.type(screen.getByLabelText("マスターパスワード"), "a long enough password");
    await userEvent.click(screen.getByRole("button", { name: "ロックを解除" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Vaultをバージョン4から5へ更新できませんでした。元のVaultは変更していません。",
    );
  });

  it("shows a copyable safe diagnostic when Android storage rejects vault creation", async () => {
    const api = buildApi({
      initialiseVault: vi.fn().mockRejectedValue(new ApiError("vault_storage_permission_denied", 500, {
        code: "vault_storage_permission_denied",
        message: "request rejected",
        detail: "the operating system denied access to the app's private storage",
      })),
    });
    render(<LockScreen exists={false} minPassphraseLength={4} version="0.13.6" onOpen={vi.fn()} api={api} />);

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
      passwordVault: vi.fn().mockResolvedValue(vaultStatus({ unlocked: false })),
    });
    const { rerender } = render(
      <LockScreen exists={false} minPassphraseLength={4} version="0.13.6" onOpen={vi.fn()} onExists={onExists} api={api} />,
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


it("creates a passwordless vault only after selecting that mode", async () => {
  const api = buildApi();
  const onOpen = vi.fn();
  render(<LockScreen exists={false} minPassphraseLength={4} api={api} onOpen={onOpen} />);
  expect(screen.getByRole("button", { name: "Create the vault" })).toBeDisabled();
  await userEvent.click(screen.getByLabelText("Use without a password"));
  expect(screen.queryByLabelText("Master password")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Create the vault" }));
  await waitFor(() => expect(api.initialiseVault).toHaveBeenCalledWith(""));
  expect(onOpen).toHaveBeenCalled();
});

it("accepts four Unicode characters and explains short-password protection", async () => {
  const api = buildApi();
  render(<LockScreen exists={false} minPassphraseLength={4} api={api} onOpen={vi.fn()} />);
  await userEvent.type(screen.getByLabelText("Master password"), "あいうえ");
  await userEvent.type(screen.getByLabelText("Confirm master password"), "あいうえ");
  expect(screen.getByText(/A short password offers limited protection/)).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Create the vault" }));
  await waitFor(() => expect(api.initialiseVault).toHaveBeenCalledWith("あいうえ"));
});

it("reopens a manually locked passwordless vault without an input field", async () => {
  const api = buildApi();
  render(<LockScreen exists passwordless api={api} onOpen={vi.fn()} />);
  expect(screen.queryByLabelText("Master password")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Open" }));
  await waitFor(() => expect(api.unlockVault).toHaveBeenCalledWith(""));
});

// scripts/android/vault-lifecycle-test.mjs は、文言ではなく、パスワード欄の数と、
// その欄と同じformのsubmitボタンで作成とロックの解除を操作する。
it.each([
  { exists: false, passwordInputs: 2 },
  { exists: true, passwordInputs: 1 },
])("keeps a single submit button in the password form when exists is $exists", ({ exists, passwordInputs }) => {
  render(
    <LanguageProvider initial="ja">
      <LockScreen exists={exists} api={buildApi()} onOpen={vi.fn()} />
    </LanguageProvider>,
  );
  const inputs = [...document.querySelectorAll("input")].filter((input) => input.type === "password");
  expect(inputs).toHaveLength(passwordInputs);
  const form = inputs[0]?.form;
  expect(form).toBeInstanceOf(HTMLFormElement);
  expect(inputs.every((input) => input.form === form)).toBe(true);
  expect(form?.querySelectorAll('button[type="submit"]')).toHaveLength(1);
});
