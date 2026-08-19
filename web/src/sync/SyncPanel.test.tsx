import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { SyncPanel } from "./SyncPanel";
import type { IntegrationsApi, PullResponse, SyncStatus } from "../api/integrations";

afterEach(() => {
  vi.restoreAllMocks();
});

const unconfigured: SyncStatus = {
  configured: false,
  keyConfigured: false,
  locked: false,
  auto: { enabled: false, phase: "idle" },
  endpoint: "",
  bucket: "",
  synced: false,
  direction: "both",
};
const configured: SyncStatus = {
  configured: true,
  keyConfigured: true,
  locked: false,
  auto: { enabled: false, phase: "idle" },
  endpoint: "https://acc.r2.cloudflarestorage.com",
  bucket: "sshc",
  synced: true,
  direction: "both",
  lastSyncedAt: "2026-08-05T00:00:00Z",
  fileCount: 7,
};

function buildApi(status: SyncStatus, pull: PullResponse, overrides: Partial<IntegrationsApi> = {}): IntegrationsApi {
  return {
    syncStatus: vi.fn().mockResolvedValue(status),
    configureSync: vi.fn().mockResolvedValue({ ...status, configured: true }),
    pushSnapshot: vi.fn().mockResolvedValue({
      status: { ...status, synced: true },
      result: {
        summary: measuredSummary,
        objectCount: 2,
        uploadedBytes: 1800,
        completedAt: "2026-08-12T01:02:04Z",
      },
    }),
    pullSnapshot: vi.fn().mockResolvedValue(pull),
    ...overrides,
  } as unknown as IntegrationsApi;
}

const measuredSummary = {
  createdAt: "2026-08-12T01:02:03Z",
  fileCount: 7,
  sourceBytes: 1200,
  snapshotBytes: 900,
};

const nothingToDo: PullResponse = {
  applied: false,
  conflicts: [],
  written: [],
  removed: [],
  summary: measuredSummary,
  downloadedBytes: 900,
  completedAt: "2026-08-12T01:02:04Z",
};

describe("SyncPanel", () => {
  it("says what travels before the form asks for anything", async () => {
    render(<SyncPanel api={buildApi(unconfigured, nothingToDo)} />);

    expect(await screen.findByText(/including your private keys/)).toBeInTheDocument();
    expect(screen.getByText(/attack that passphrase offline/)).toBeInTheDocument();
  });

  it("configures a bucket and clears the credentials from the form", async () => {
    // 送信後もフィールドに残されたシークレットは、理由もなく DOM に
    // 座り続けるシークレットだ。
    const api = buildApi(unconfigured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.type(await screen.findByLabelText("Endpoint"), "https://acc.r2.cloudflarestorage.com");
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(screen.getByLabelText("Secret access key"), "the-secret");
    await userEvent.click(await screen.findByRole("button", { name: "Use this bucket" }));

    await waitFor(() =>
      expect(api.configureSync).toHaveBeenCalledWith({
        endpoint: "https://acc.r2.cloudflarestorage.com",
        bucket: "sshc",
        path: "",
        region: "",
        accessKeyId: "AKID",
        secretAccessKey: "the-secret",
        direction: "both",
      }),
    );
    await waitFor(() => expect(screen.queryByLabelText("Secret access key")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Edit bucket settings" })).toBeInTheDocument();
    expect(document.body.textContent ?? "").not.toContain("the-secret");
  });

  it("keeps configured credentials collapsed until they need editing", async () => {
    render(<SyncPanel api={buildApi(configured, nothingToDo)} />);

    expect(await screen.findByText("https://acc.r2.cloudflarestorage.com/sshc")).toBeInTheDocument();
    expect(screen.queryByLabelText("Secret access key")).not.toBeInTheDocument();
    await userEvent.click(await screen.findByRole("button", { name: "Edit bucket settings" }));
    expect(screen.getByLabelText("Endpoint")).toHaveValue("https://acc.r2.cloudflarestorage.com");
    expect(screen.getByLabelText("Bucket name")).toHaveValue("sshc");
    expect(screen.getByLabelText("Secret access key")).toHaveValue("");
  });

  // リージョンは署名スコープに入る。R2 の "auto" と本物の AWS のバケットの
  // リージョンでは違うので、空欄のままサーバーの既定に任せるのではなく、打ち込んだ
  // ものがそのまま届かなければならない。
  it("sends the region it was given", async () => {
    const api = buildApi(unconfigured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.type(await screen.findByLabelText("Endpoint"), "https://s3.eu-west-2.amazonaws.com");
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Region"), "eu-west-2");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(screen.getByLabelText("Secret access key"), "the-secret");
    await userEvent.click(await screen.findByRole("button", { name: "Use this bucket" }));

    await waitFor(() =>
      expect(api.configureSync).toHaveBeenCalledWith(
        expect.objectContaining({ region: "eu-west-2" }),
      ),
    );
  });

  it("offers no push or pull until a bucket and a passphrase are given", async () => {
    render(<SyncPanel api={buildApi(unconfigured, nothingToDo)} />);

    expect(await screen.findByRole("button", { name: "Push this workspace" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Check for changes" })).toBeDisabled();
  });

  it("previews before it applies", async () => {
    // 最初の押下で書き込んでしまう pull は、このアプリケーションで
    // プレビューを飛ばす唯一の書き込みになってしまう。
    const api = buildApi(configured, {
      ...nothingToDo,
      applied: false,
      conflicts: [],
      written: ["config", "connections/work/lon.conf"],
      removed: ["connections/old.conf"],
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Check for changes" }));

    await waitFor(() => expect(api.pullSnapshot).toHaveBeenCalledWith(false, undefined));
    expect(await screen.findByText("connections/work/lon.conf")).toBeInTheDocument();
    expect(screen.getByText("connections/old.conf")).toBeInTheDocument();

    // 消すものがあるので、適用の前に一度そう言わせる。
    await userEvent.click(screen.getByRole("checkbox", { name: /overwrites files in ~\/.ssh/i }));
    await userEvent.click(await screen.findByRole("button", { name: "Apply the snapshot" }));
    await waitFor(() => expect(api.pullSnapshot).toHaveBeenLastCalledWith(true, undefined));
  });

  it("shows a conflict and refuses to apply it", async () => {
    // 同じブロックを両方が変更した 2 つの設定に正しいマージはないので、
    // これはファイルを名指して止まる。
    const api = buildApi(configured, {
      ...nothingToDo,
      applied: false,
      conflicts: [{ path: "config", changedHere: true, changedThere: true }],
      written: [],
      removed: [],
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Check for changes" }));

    expect(await screen.findByText(/changed here and on the other machine/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Apply the snapshot" })).toBeDisabled();
  });

  it("says so when there is nothing to do", async () => {
    const api = buildApi(configured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Check for changes" }));

    expect(await screen.findByText(/already matches the snapshot/)).toBeInTheDocument();
  });

  it("shows the measured push result instead of only saying it succeeded", async () => {
    const api = buildApi(configured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Push this workspace" }));

    expect(await screen.findByRole("heading", { name: "This push" })).toBeInTheDocument();
    expect(screen.getByText("7 files · 1.2 kB")).toBeInTheDocument();
    expect(screen.getByText("S3 transfer 1.8 kB (2 objects, history + live)")).toBeInTheDocument();
  });

  it("shows preview and apply as two separately measured downloads", async () => {
    const preview = { ...nothingToDo, written: ["config"] };
    const applied = { ...preview, applied: true, completedAt: "2026-08-12T01:03:00Z" };
    const api = buildApi(configured, preview, {
      pullSnapshot: vi.fn().mockResolvedValueOnce(preview).mockResolvedValueOnce(applied),
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Check for changes" }));
    expect(await screen.findByRole("heading", { name: "Pull preview" })).toBeInTheDocument();
    expect(screen.getByText("Downloaded 900 B · 1.2 kB after opening")).toBeInTheDocument();

    await userEvent.click(await screen.findByRole("button", { name: "Apply the snapshot" }));
    expect(await screen.findByRole("heading", { name: "Apply result" })).toBeInTheDocument();
    expect(screen.getByText("Downloaded again for apply: 900 B")).toBeInTheDocument();
  });

  it("shows a persisted operation as the previous success after reload", async () => {
    const lastOperation = {
      kind: "push" as const,
      summary: measuredSummary,
      objectCount: 2,
      uploadedBytes: 1800,
      completedAt: "2026-08-12T01:02:04Z",
    };
    render(<SyncPanel api={buildApi({ ...configured, lastOperation }, nothingToDo)} />);

    expect(await screen.findByRole("heading", { name: "Previous success" })).toBeInTheDocument();
    expect(screen.getByText("S3 transfer 1.8 kB (2 objects, history + live)")).toBeInTheDocument();
  });

  it("keeps the previous success separate when a later push fails partway", async () => {
    const lastOperation = {
      kind: "push" as const,
      summary: measuredSummary,
      objectCount: 2,
      uploadedBytes: 1800,
      completedAt: "2026-08-12T01:02:04Z",
    };
    const api = buildApi({ ...configured, lastOperation }, nothingToDo, {
      pushSnapshot: vi.fn().mockRejectedValue(new ApiError("sync_remote_moved", 409, null)),
    });
    render(<SyncPanel api={api} />);

    expect(await screen.findByRole("heading", { name: "Previous success" })).toBeInTheDocument();
    await userEvent.click(await screen.findByRole("button", { name: "Push this workspace" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/live snapshot was not updated/i);
    expect(screen.getByRole("alert")).toHaveTextContent(/dated history copy.*may remain/i);
    expect(screen.getByRole("heading", { name: "Previous success" })).toBeInTheDocument();
  });

  it("sends the chosen direction with the bucket", async () => {
    const api = buildApi(unconfigured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.type(await screen.findByLabelText("Endpoint"), "https://acc.r2.cloudflarestorage.com");
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(screen.getByLabelText("Secret access key"), "the-secret");
    await userEvent.selectOptions(screen.getByLabelText("Direction"), "pull");
    await userEvent.click(await screen.findByRole("button", { name: "Use this bucket" }));

    await waitFor(() =>
      expect(api.configureSync).toHaveBeenCalledWith(
        expect.objectContaining({ direction: "pull" }),
      ),
    );
  });

  it("offers no push on a machine set to receive only, and says why", async () => {
    const api = buildApi({ ...configured, direction: "pull" }, nothingToDo);
    render(<SyncPanel api={api} />);

    expect(await screen.findByRole("button", { name: "Push this workspace" })).toBeDisabled();
    // 理由はコントロールの隣に立つ。無効化されたボタンの隣に何もなければ、
    // それは設定ではなく不具合に見えてしまう。
    expect(screen.getByText(/Set to receive only/)).toBeInTheDocument();
  });

  it("offers no apply on a machine set to send only, but still shows what would change", async () => {
    const api = buildApi({ ...configured, direction: "push" }, {
      ...nothingToDo,
      applied: false,
      conflicts: [],
      written: ["config"],
      removed: [],
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Check for changes" }));

    // 見ることは動かすことではない。適用してはならないマシンでも、
    // 自分がどれだけ遅れているかを知ることは許される。
    expect(await screen.findByText("config")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Apply the snapshot" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Push this workspace" })).toBeEnabled();
  });

  it("reports a refused push instead of claiming success", async () => {
    const api = buildApi(configured, nothingToDo, {
      pushSnapshot: vi.fn().mockRejectedValue(new Error("sync_remote_moved")),
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Push this workspace" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/pull first|could not be pushed/i);
  });
  // 設定は今やマスターパスワードで封印されているので、閉じた vault は
  // このフォームを埋められない。それでも空のフォームを見せれば、
  // 「バケットが消えた」と読め、ユーザーにアクセスキーの再入力を促してしまう。
  it("asks for the master password rather than showing an empty bucket form", async () => {
    const api = buildApi({ ...unconfigured, locked: true }, nothingToDo);
    render(<SyncPanel api={api} />);

    expect(await screen.findByLabelText("Master password")).toBeInTheDocument();
    expect(screen.queryByLabelText("Access key ID")).not.toBeInTheDocument();
    expect(screen.getByText(/sealed with the master password/i)).toBeInTheDocument();
  });

  it("opens the vault in place and reads the settings back", async () => {
    // 起動時には何も尋ねられない: これは画面が答えを必要とする瞬間に
    // 自分自身のために尋ねているのだ。
    const syncStatus = vi
      .fn()
      .mockResolvedValueOnce({ ...unconfigured, locked: true })
      .mockResolvedValue(configured);
    const api = buildApi(unconfigured, nothingToDo, {
      syncStatus,
      unlockVault: vi.fn().mockResolvedValue({ exists: true, unlocked: true, biometric: { available: false, enabled: false }, aliases: [], dedicatedKeyPassphrases: [] }),
    });
    render(<SyncPanel api={api} />);

    await userEvent.type(await screen.findByLabelText("Master password"), "the master password");
    await userEvent.click(await screen.findByRole("button", { name: "Unlock" }));

    await waitFor(() => expect(api.unlockVault).toHaveBeenCalledWith("the master password"));
    expect(await screen.findByText("https://acc.r2.cloudflarestorage.com/sshc")).toBeInTheDocument();
  });
  // 封をしているのはマスターパスワードではなく、保管庫の中の鍵である。開かない
  // ときに言うべきことは「パスワードが違う」ではなく、「このバケットは、この
  // マシンが鍵を持つ前に書かれたものかもしれない」である。
  it("points at the key, not the master password, when the snapshot does not open", async () => {
    const api = buildApi(configured, nothingToDo, {
      pushSnapshot: vi.fn().mockRejectedValue(new ApiError("wrong_passphrase", 403, null)),
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Push this workspace" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/stored key does not open/i);
  });

  // **鍵は一度しか見せない。** リモートを開ける値を、画面を開き直すたびに配る
  // 画面にしてはならない。
  it("shows a generated key once and never asks for it again", async () => {
    const setSyncKey = vi.fn().mockResolvedValue({ key: "AB12-CD34-EF56-GH78-JK90-MN12" });
    const syncStatus = vi
      .fn()
      .mockResolvedValueOnce({ ...configured, keyConfigured: false })
      .mockResolvedValue(configured);
    const api = buildApi(configured, nothingToDo, { setSyncKey, syncStatus });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Create a key" }));

    await waitFor(() => expect(setSyncKey).toHaveBeenCalledWith(undefined));
    expect(await screen.findByText("AB12-CD34-EF56-GH78-JK90-MN12")).toBeInTheDocument();
    // そして押す前に、押した人が打つ欄はどこにも無い。
    expect(screen.queryByLabelText("Key")).not.toBeInTheDocument();
  });

  // 自分で決める道も残っている。決めたものは表示しない——打った人はすでに知って
  // いる。
  it("takes a key the person chose without echoing it back", async () => {
    const setSyncKey = vi.fn().mockResolvedValue({ key: "a key chosen by hand" });
    const api = buildApi({ ...configured, keyConfigured: false }, nothingToDo, { setSyncKey });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByLabelText("Choose the key myself"));
    await userEvent.type(screen.getByLabelText("Key"), "a key chosen by hand");
    await userEvent.click(await screen.findByRole("button", { name: "Create a key" }));

    await waitFor(() => expect(setSyncKey).toHaveBeenCalledWith("a key chosen by hand"));
    expect(screen.queryByText("a key chosen by hand")).not.toBeInTheDocument();
  });

  // 鍵が無ければ押せない。押せてしまえば、リモートには誰も開けない書庫が残る。
  it("offers no push or pull until a key exists", async () => {
    const api = buildApi({ ...configured, keyConfigured: false }, nothingToDo);
    render(<SyncPanel api={api} />);

    expect(await screen.findByRole("button", { name: "Push this workspace" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Check for changes" })).toBeDisabled();
  });

  // 設定は保持される前に試されるので、画面は何も保存されなかったと
  // 言わなければならない——さもないと、ユーザーは保存されたと信じたまま去ってしまう。
  it("says nothing was saved when the bucket did not answer", async () => {
    const api = buildApi(unconfigured, nothingToDo, {
      configureSync: vi.fn().mockRejectedValue(new ApiError("bucket_refused", 502, null)),
    });
    render(<SyncPanel api={api} />);

    await userEvent.type(await screen.findByLabelText("Endpoint"), "https://acc.r2.cloudflarestorage.com");
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(screen.getByLabelText("Secret access key"), "the-secret");
    await userEvent.click(await screen.findByRole("button", { name: "Use this bucket" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/Nothing was saved/i);
  });

  it("explains an endpoint that carries a path instead of just refusing it", async () => {
    const api = buildApi(unconfigured, nothingToDo, {
      configureSync: vi.fn().mockRejectedValue(new ApiError("endpoint_must_have_no_path", 400, null)),
    });
    render(<SyncPanel api={api} />);

    await userEvent.type(await screen.findByLabelText("Endpoint"), "https://acc.r2.cloudflarestorage.com/sshc");
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(screen.getByLabelText("Secret access key"), "the-secret");
    await userEvent.click(await screen.findByRole("button", { name: "Use this bucket" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/no bucket name and no path/i);
  });
  // **消すときだけ、もう一段いる。** 置き換えは History から戻せるが、消えた
  // ファイルは画面から消える——押した人が中身を一度も見ていないこともある。
  it("will not apply a pull that removes files until it is told to go ahead", async () => {
    const removing = {
      applied: false,
      summary: { createdAt: "2026-08-12T01:30:00Z", fileCount: 2, sourceBytes: 10, snapshotBytes: 20 },
      downloadedBytes: 20,
      completedAt: "2026-08-12T01:31:00Z",
      conflicts: [],
      written: [],
      removed: ["~/.ssh/connections/old.conf"],
    };
    const api = buildApi(configured, removing);
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Check for changes" }));
    const apply = await screen.findByRole("button", { name: "Apply the snapshot" });
    expect(apply).toBeDisabled();

    await userEvent.click(screen.getByRole("checkbox", { name: /overwrites files in ~\/.ssh/i }));
    expect(apply).toBeEnabled();
  });

  // 消さない pull を、余計な同意で止めない。
  it("applies a pull that only writes without asking again", async () => {
    const api = buildApi(configured, {
      applied: false,
      summary: { createdAt: "2026-08-12T01:30:00Z", fileCount: 2, sourceBytes: 10, snapshotBytes: 20 },
      downloadedBytes: 20,
      completedAt: "2026-08-12T01:31:00Z",
      conflicts: [],
      written: ["~/.ssh/config"],
      removed: [],
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Check for changes" }));
    expect(await screen.findByRole("button", { name: "Apply the snapshot" })).toBeEnabled();
    expect(screen.queryByRole("checkbox", { name: /overwrites files/i })).not.toBeInTheDocument();
  });

  // **押さなくても進むが、黙って壊しはしない。** 巡回が止まったとき、画面は
  // 何を待っているのかを言う——「同期に失敗しました」では、どこを見ればよいか
  // 分からない。
  it("says what the loop is waiting for instead of only that it stopped", async () => {
    const api = buildApi(
      { ...configured, auto: { enabled: true, phase: "blocked", detail: "removals", at: "2026-08-18T00:00:00Z" } },
      nothingToDo,
    );
    render(<SyncPanel api={api} />);

    expect(await screen.findByText(/would remove files from this machine/i)).toBeInTheDocument();
  });

  // 切ったことは保管庫に残るので、押した結果は status で返ってくる。
  it("turns the loop on and keeps what the server answered", async () => {
    const setAutoSync = vi
      .fn()
      .mockResolvedValue({ ...configured, auto: { enabled: true, phase: "idle" } });
    const api = buildApi(configured, nothingToDo, { setAutoSync });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("checkbox", { name: /Keep this machine in sync/i }));

    await waitFor(() => expect(setAutoSync).toHaveBeenCalledWith(true));
    expect(await screen.findByRole("checkbox", { name: /Keep this machine in sync/i })).toBeChecked();
  });

  // 巡回が入っていなければ「今すぐ」は押せない。押せてしまえば、起きていない
  // ことを起きたと言うことになる。
  it("offers no manual cycle while the loop is off", async () => {
    const api = buildApi(configured, nothingToDo);
    render(<SyncPanel api={api} />);

    expect(await screen.findByRole("button", { name: "Sync now" })).toBeDisabled();
  });

  // **選ぶ道が無ければ、自分の設定を持ったまま繋いだ 2 台目は一度も同期を
  // 終えられない。** 選んでも書く前に同じプレビューが出る——寄せ先は適用では
  // なく計画を変える。
  it("offers both sides of a conflict and previews the choice before applying it", async () => {
    const conflicted = {
      applied: false,
      summary: { createdAt: "2026-08-12T01:30:00Z", fileCount: 1, sourceBytes: 10, snapshotBytes: 20 },
      downloadedBytes: 20,
      completedAt: "2026-08-12T01:31:00Z",
      conflicts: [{ path: "config", changedHere: true, changedThere: true }],
      written: [],
      removed: [],
    };
    const resolved = { ...conflicted, conflicts: [], written: ["config"] };
    const pullSnapshot = vi
      .fn()
      .mockResolvedValueOnce(conflicted)
      .mockResolvedValue(resolved);
    const api = buildApi(configured, conflicted, { pullSnapshot });
    render(<SyncPanel api={api} />);

    await userEvent.click(await screen.findByRole("button", { name: "Check for changes" }));
    await userEvent.click(await screen.findByRole("button", { name: "Take the other machine's version" }));

    // 取り直したプレビューは、寄せ先を伴っている。まだ何も書いていない。
    await waitFor(() => expect(pullSnapshot).toHaveBeenLastCalledWith(false, "remote"));
    await userEvent.click(await screen.findByRole("button", { name: "Apply the snapshot" }));
    await waitFor(() => expect(pullSnapshot).toHaveBeenLastCalledWith(true, "remote"));
  });
});
