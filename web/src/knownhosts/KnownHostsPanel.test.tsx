import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KnownHostsPanel } from "./KnownHostsPanel";
import { ApiError } from "../api/client";
import { LanguageProvider } from "../i18n/context";
import { ToastProvider, toastVisibleMs } from "../ui/ToastProvider";
import type { KnownHostCandidate } from "../api/knownHosts";
import type { KnownHostsApi, KnownHostsResponse } from "../api/knownHosts";

afterEach(() => {
  vi.restoreAllMocks();
});

const entry = {
  line: 2,
  digest: "a".repeat(64),
  marker: "",
  hosts: ["bastion.example.com", "203.0.113.10"],
  hashed: false,
  keyType: "ssh-ed25519",
  fingerprint: "SHA256:bytFrSjxj2qRszG8sHhWN+YO3b9vDSU3gQtMorwKpEs",
  comment: "admin@example",
};

const candidateFingerprint = "SHA256:bytFrSjxj2qRszG8sHhWN+YO3b9vDSU3gQtMorwKpEs";
const candidateKey = "AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS";

const candidate: KnownHostCandidate = {
  host: "new.example.com",
  port: 22,
  keyType: "ssh-ed25519",
  key: candidateKey,
  fingerprint: candidateFingerprint,
  verified: false,
};

// engine の notice は英語の定数で、画面は読まない。
const engineNotice = "Engine notice that the screen does not show.";

function scanResult(...candidates: KnownHostCandidate[]) {
  return { notice: engineNotice, candidates };
}

function buildApi(overrides: Partial<KnownHostsApi> = {}): KnownHostsApi {
  return {
    knownHosts: vi.fn().mockResolvedValue({ path: "~/.ssh/known_hosts", entries: [entry] }),
    deleteKnownHosts: vi.fn().mockResolvedValue({ changed: true, transactionId: "tx-1" }),
    scanKnownHosts: vi.fn().mockResolvedValue(scanResult(candidate)),
    addKnownHost: vi.fn().mockResolvedValue({ changed: true, transactionId: "tx-2" }),
    ...overrides,
  };
}

async function openAddForm(api: KnownHostsApi) {
  render(<ToastProvider><KnownHostsPanel api={api} /></ToastProvider>);
  await userEvent.type(await screen.findByLabelText("Host to scan"), "new.example.com");
  await userEvent.click(screen.getByRole("button", { name: "Scan" }));
  const row = await screen.findByRole("row", { name: /new\.example\.com/ });
  await userEvent.click(within(row).getByRole("button", { name: "Add" }));
}

const fingerprintField = /Fingerprint verified through another channel/;
const acknowledgement = /accept the risk/;

describe("KnownHostsPanel", () => {
  it("lists entries with their fingerprints", async () => {
    render(<KnownHostsPanel api={buildApi()} />);

    const row = await screen.findByRole("row", { name: /bastion\.example\.com/ });
    expect(within(row).getByText(/SHA256:bytFrSjx/)).toBeInTheDocument();
  });

  it("keeps the listing of the latest query when an earlier search answers last", async () => {
    const path = "~/.ssh/known_hosts";
    const web0 = { ...entry, line: 3, hosts: ["web0.example.com"] };
    const web01 = { ...entry, line: 4, hosts: ["web01.example.com"] };
    let answerWeb0: (listing: KnownHostsResponse) => void = () => undefined;
    const knownHosts = vi.fn((query: string): Promise<KnownHostsResponse> => {
      if (query === "web0") return new Promise((resolve) => { answerWeb0 = resolve; });
      if (query === "web01") return Promise.resolve({ path, entries: [web01] });
      return Promise.resolve({ path, entries: [entry] });
    });
    render(<KnownHostsPanel api={buildApi({ knownHosts })} />);
    await screen.findByRole("row", { name: /bastion\.example\.com/ });

    await userEvent.type(screen.getByLabelText("Search"), "web01");
    await screen.findByRole("row", { name: /web01\.example\.com/ });
    await act(async () => answerWeb0({ path, entries: [web0] }));

    expect(screen.getByRole("row", { name: /web01\.example\.com/ })).toBeInTheDocument();
    expect(screen.queryByRole("row", { name: /web0\.example\.com/ })).toBeNull();
  });

  it("sorts trusted entries from every data-column header", async () => {
    const alpha = {
      ...entry,
      line: 3,
      digest: "b".repeat(64),
      hosts: ["alpha.example.com"],
      keyType: "ecdsa-sha2-nistp256",
      fingerprint: "SHA256:alpha",
    };
    const api = buildApi({
      knownHosts: vi.fn().mockResolvedValue({ path: "~/.ssh/known_hosts", entries: [entry, alpha] }),
    });
    render(<KnownHostsPanel api={api} />);

    const table = await screen.findByRole("table", { name: "~/.ssh/known_hosts" });
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("alpha.example.com");
    await userEvent.click(within(table).getByRole("button", { name: /Host.*sort descending/ }));
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("bastion.example.com");
    await userEvent.click(within(table).getByRole("button", { name: /Key type.*sort ascending/ }));
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("ecdsa-sha2-nistp256");
    await userEvent.click(within(table).getByRole("button", { name: /Fingerprint.*sort ascending/ }));
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("SHA256:alpha");
  });

  it("confirms before deleting and sends the digest of the line that was shown", async () => {
    const api = buildApi();
    render(<KnownHostsPanel api={api} />);

    const row = await screen.findByRole("row", { name: /bastion\.example\.com/ });
    await userEvent.click(within(row).getByRole("button", { name: "Delete" }));
    expect(api.deleteKnownHosts).not.toHaveBeenCalled();

    await userEvent.click(await screen.findByRole("button", { name: "Confirm delete" }));
    await waitFor(() =>
      expect(api.deleteKnownHosts).toHaveBeenCalledWith([{ line: 2, digest: "a".repeat(64) }], "~/.ssh/known_hosts"),
    );
  });

  it("warns in the screen's language that a scan does not prove the host's identity", async () => {
    render(<LanguageProvider initial="ja"><KnownHostsPanel api={buildApi()} /></LanguageProvider>);

    await userEvent.type(await screen.findByLabelText("スキャンするホスト"), "new.example.com");
    await userEvent.click(screen.getByRole("button", { name: "スキャン" }));

    expect(await screen.findByText(/ホストの身元は証明されません/)).toBeInTheDocument();
    expect(screen.queryByText(engineNotice)).not.toBeInTheDocument();
  });

  it("marks every scanned key unverified and refuses to call it trusted", async () => {
    const api = buildApi();
    render(<KnownHostsPanel api={api} />);

    await userEvent.type(await screen.findByLabelText("Host to scan"), "new.example.com");
    await userEvent.click(screen.getByRole("button", { name: "Scan" }));

    expect(await screen.findByText(/does not prove the host's identity/)).toBeInTheDocument();
    const candidate = await screen.findByRole("row", { name: /new\.example\.com/ });
    expect(within(candidate).getByText("unverified")).toBeInTheDocument();
    expect(within(candidate).queryByText("verified")).not.toBeInTheDocument();
  });

  it("never labels a scanned key verified, even when the response claims it is", async () => {
    const api = buildApi({
      scanKnownHosts: vi.fn().mockResolvedValue(scanResult({ ...candidate, verified: true })),
    });
    render(<KnownHostsPanel api={api} />);

    await userEvent.type(await screen.findByLabelText("Host to scan"), "new.example.com");
    await userEvent.click(screen.getByRole("button", { name: "Scan" }));

    const row = await screen.findByRole("row", { name: /new\.example\.com/ });
    expect(within(row).getByText("unverified")).toBeInTheDocument();
    expect(within(row).queryByText("verified")).not.toBeInTheDocument();
    expect(within(row).queryByText(/trusted/i)).not.toBeInTheDocument();
  });

  it("keeps the add control unavailable until a fingerprint is typed or the risk is acknowledged", async () => {
    const api = buildApi();
    await openAddForm(api);

    const add = screen.getByRole("button", { name: "Add to known_hosts" });
    expect(add).toBeDisabled();

    await userEvent.type(screen.getByLabelText(fingerprintField), candidateFingerprint);
    expect(add).toBeEnabled();

    await userEvent.clear(screen.getByLabelText(fingerprintField));
    expect(add).toBeDisabled();

    await userEvent.click(screen.getByLabelText(acknowledgement));
    expect(add).toBeEnabled();
    expect(api.addKnownHost).not.toHaveBeenCalled();
  });

  it("refuses a typed fingerprint that does not match the candidate and shows the mismatch", async () => {
    const api = buildApi();
    await openAddForm(api);

    await userEvent.type(screen.getByLabelText(fingerprintField), "SHA256:notTheKeyThatWasScanned");
    await userEvent.click(screen.getByRole("button", { name: "Add to known_hosts" }));

    expect(api.addKnownHost).not.toHaveBeenCalled();
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("does not match");
    expect(alert).toHaveTextContent("SHA256:notTheKeyThatWasScanned");
    expect(alert).toHaveTextContent(candidateFingerprint);
  });

  it("sends the typed fingerprint rather than an acknowledgement when the key was proven", async () => {
    const api = buildApi();
    await openAddForm(api);

    await userEvent.type(screen.getByLabelText(fingerprintField), `  ${candidateFingerprint}  `);
    await userEvent.click(screen.getByRole("button", { name: "Add to known_hosts" }));

    await waitFor(() =>
      expect(api.addKnownHost).toHaveBeenCalledWith(
        { host: "new.example.com", port: 22, keyType: "ssh-ed25519", key: candidateKey },
        candidateFingerprint,
        false,
      ),
    );
    expect(await screen.findByText("Added new.example.com.")).toBeInTheDocument();
  });

  it("adds an unverified key only on an explicit acknowledgement and forgets it afterwards", async () => {
    const api = buildApi();
    await openAddForm(api);

    await userEvent.click(screen.getByLabelText(acknowledgement));
    await userEvent.click(screen.getByRole("button", { name: "Add to known_hosts" }));

    await waitFor(() =>
      expect(api.addKnownHost).toHaveBeenCalledWith(
        { host: "new.example.com", port: 22, keyType: "ssh-ed25519", key: candidateKey },
        "",
        true,
      ),
    );
    const row = await screen.findByRole("row", { name: /new\.example\.com/ });
    await userEvent.click(within(row).getByRole("button", { name: "Add" }));
    expect(screen.getByLabelText(acknowledgement)).not.toBeChecked();
    expect(screen.getByRole("button", { name: "Add to known_hosts" })).toBeDisabled();
  });

  it("does not carry a fingerprint typed for one key over to another", async () => {
    const other: KnownHostCandidate = {
      ...candidate,
      host: "other.example.com",
      keyType: "ssh-rsa",
      key: "AAAAB3NzaC1yc2EAAAADAQABAAAAgQDother",
      fingerprint: "SHA256:aDifferentKeyEntirely",
    };
    const api = buildApi({ scanKnownHosts: vi.fn().mockResolvedValue(scanResult(candidate, other)) });
    await openAddForm(api);

    await userEvent.type(screen.getByLabelText(fingerprintField), candidateFingerprint);
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    const otherRow = await screen.findByRole("row", { name: /aDifferentKeyEntirely/ });
    await userEvent.click(within(otherRow).getByRole("button", { name: "Add" }));

    expect(screen.getByLabelText(fingerprintField)).toHaveValue("");
    expect(screen.getByRole("button", { name: "Add to known_hosts" })).toBeDisabled();
  });

  it("surfaces the refusal code from the add endpoint and stores nothing", async () => {
    const frameworkGlobals = ["IS_REACT_ACT_ENVIRONMENT"];
    const globalsBefore = Object.keys(window);
    const api = buildApi({
      addKnownHost: vi.fn().mockRejectedValue(
        new ApiError("unverified_candidate", 409, {
          code: "unverified_candidate",
          message: "a scanned key needs a matching fingerprint or an explicit acknowledgement",
        }),
      ),
    });
    await openAddForm(api);

    await userEvent.click(screen.getByLabelText(acknowledgement));
    await userEvent.click(screen.getByRole("button", { name: "Add to known_hosts" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("unverified_candidate");
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
    const added = Object.keys(window).filter(
      (key) => !globalsBefore.includes(key) && !frameworkGlobals.includes(key),
    );
    expect(added).toEqual([]);
  });

  it("says the entry was removed for a moment, without leaving the result on the screen", async () => {
    const api = buildApi();
    render(<ToastProvider><KnownHostsPanel api={api} /></ToastProvider>);

    const row = await screen.findByRole("row", { name: /bastion\.example\.com/ });
    await userEvent.click(within(row).getByRole("button", { name: "Delete" }));
    await userEvent.click(await screen.findByRole("button", { name: "Confirm delete" }));

    expect(await screen.findByText("Removed one entry.")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Removed one entry.")).not.toBeInTheDocument(), { timeout: toastVisibleMs + 1000 });
  });

  it("reports a refused deletion without claiming the entry was removed", async () => {
    const api = buildApi({
      deleteKnownHosts: vi.fn().mockRejectedValue(new Error("api_mutation_failed")),
    });
    render(<KnownHostsPanel api={api} />);

    const row = await screen.findByRole("row", { name: /bastion\.example\.com/ });
    await userEvent.click(within(row).getByRole("button", { name: "Delete" }));
    await userEvent.click(await screen.findByRole("button", { name: "Confirm delete" }));

    const dialog = screen.getByRole("dialog");
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(/could not/i);
    expect(within(dialog).getByRole("button", { name: "Confirm delete" })).toBeEnabled();
  });

  it("sends one deletion however often confirm is pressed while it is pending", async () => {
    const deleteKnownHosts = vi.fn(() => new Promise<never>(() => undefined));
    render(<KnownHostsPanel api={buildApi({ deleteKnownHosts })} />);

    const row = await screen.findByRole("row", { name: /bastion\.example\.com/ });
    await userEvent.click(within(row).getByRole("button", { name: "Delete" }));
    const confirm = await screen.findByRole("button", { name: "Confirm delete" });
    await userEvent.click(confirm);
    await userEvent.click(confirm);

    expect(deleteKnownHosts).toHaveBeenCalledTimes(1);
    expect(confirm).toBeDisabled();
  });

  it("says the file is loading instead of counting zero entries before the first listing arrives", async () => {
    let answer: (listing: KnownHostsResponse) => void = () => undefined;
    const knownHosts = vi.fn(() => new Promise<KnownHostsResponse>((resolve) => {
      answer = resolve;
    }));
    render(<KnownHostsPanel api={buildApi({ knownHosts })} />);

    expect(screen.getByRole("status")).toHaveTextContent("Loading known_hosts");
    expect(screen.queryByText("Trusted entries")).not.toBeInTheDocument();
    await act(async () => answer({ path: "~/.ssh/known_hosts", entries: [entry] }));
    expect(await screen.findByRole("row", { name: /bastion\.example\.com/ })).toBeInTheDocument();
    expect(screen.getByText("Trusted entries").nextSibling).toHaveTextContent("1");
  });

  it("shows no counts for a known_hosts it could not read", async () => {
    render(<KnownHostsPanel api={buildApi({ knownHosts: vi.fn().mockRejectedValue(new Error("unreadable")) })} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("The known_hosts file could not be read.");
    expect(screen.queryByText("Trusted entries")).not.toBeInTheDocument();
  });

  it("keeps the scan form on screen while scanning after the file could not be read", async () => {
    const user = userEvent.setup();
    const scanKnownHosts = vi.fn(() => new Promise<ReturnType<typeof scanResult>>(() => undefined));
    render(<KnownHostsPanel api={buildApi({ knownHosts: vi.fn().mockRejectedValue(new Error("unreadable")), scanKnownHosts })} />);

    const host = await screen.findByLabelText("Host to scan");
    await user.type(host, "new.example.com");
    await user.click(screen.getByRole("button", { name: "Scan" }));

    expect(host).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Scan" })).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByText("Loading known_hosts…")).not.toBeInTheDocument();
  });

  it("shows the scan working until the host answers and scans once", async () => {
    const user = userEvent.setup();
    let answer: () => void = () => undefined;
    const scanKnownHosts = vi.fn(() => new Promise<ReturnType<typeof scanResult>>((resolve) => {
      answer = () => resolve(scanResult(candidate));
    }));
    render(<KnownHostsPanel api={buildApi({ scanKnownHosts })} />);

    await user.type(await screen.findByLabelText("Host to scan"), "new.example.com");
    const scan = screen.getByRole("button", { name: "Scan" });
    await user.click(scan);

    expect(scan).toHaveAttribute("aria-busy", "true");
    await user.click(scan);
    expect(scanKnownHosts).toHaveBeenCalledTimes(1);
    await act(async () => answer());
    expect(await screen.findByRole("row", { name: /new\.example\.com/ })).toBeInTheDocument();
    expect(scan).not.toHaveAttribute("aria-busy");
  });

  it("shows adding to known_hosts working and holds the form until the file is written", async () => {
    const user = userEvent.setup();
    let answer: () => void = () => undefined;
    const addKnownHost = vi.fn(() => new Promise<{ changed: boolean; transactionId: string }>((resolve) => {
      answer = () => resolve({ changed: true, transactionId: "tx-2" });
    }));
    const api = buildApi({ addKnownHost });
    await openAddForm(api);

    await user.click(screen.getByLabelText(acknowledgement));
    const add = screen.getByRole("button", { name: "Add to known_hosts" });
    await user.click(add);

    expect(add).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    await user.click(add);
    expect(addKnownHost).toHaveBeenCalledTimes(1);
    await act(async () => answer());
    expect(await screen.findByText(/Added new\.example\.com/)).toBeInTheDocument();
  });

  it("says a symlinked known_hosts is why it cannot be listed and that ssh can add a host", async () => {
    const symlinked = new ApiError("known_hosts_symlink", 403, {
      code: "known_hosts_symlink",
      message: "known_hosts_symlink",
    });
    const api = buildApi({ knownHosts: vi.fn().mockRejectedValue(symlinked) });
    render(<KnownHostsPanel api={api} />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("~/.ssh/known_hosts is a symbolic link");
    expect(alert).toHaveTextContent("connect to the host once with ssh");
  });

  it("gives the same reason when a key cannot be added to a symlinked known_hosts", async () => {
    const symlinked = new ApiError("known_hosts_symlink", 403, {
      code: "known_hosts_symlink",
      message: "known_hosts_symlink",
    });
    const api = buildApi({ addKnownHost: vi.fn().mockRejectedValue(symlinked) });
    await openAddForm(api);

    await userEvent.click(screen.getByLabelText(acknowledgement));
    await userEvent.click(screen.getByRole("button", { name: "Add to known_hosts" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("~/.ssh/known_hosts is a symbolic link");
    expect(alert).not.toHaveTextContent("known_hosts_symlink");
  });
});

describe("where the scan control sits", () => {
  it("puts the host to scan above the search box and the listing", async () => {
    render(<KnownHostsPanel api={buildApi()} />);
    await screen.findByRole("row", { name: /bastion\.example\.com/ });

    const scan = screen.getByLabelText("Host to scan");
    const search = screen.getByLabelText("Search");
    const listing = screen.getByRole("table", { name: "~/.ssh/known_hosts" });

    expect(scan.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(scan.compareDocumentPosition(listing) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("keeps the scanned candidates with the control that asked for them", async () => {
    const user = userEvent.setup();
    render(<KnownHostsPanel api={buildApi()} />);
    await screen.findByRole("row", { name: /bastion\.example\.com/ });

    await user.type(screen.getByLabelText("Host to scan"), "nas.example.com");
    await user.click(screen.getByRole("button", { name: "Scan" }));

    const candidates = await screen.findByRole("table", { name: "Scan candidates" });
    const search = screen.getByLabelText("Search");
    expect(candidates.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});
