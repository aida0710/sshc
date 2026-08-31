import { beforeEach, describe, expect, it, vi } from "vitest";
import { bootstrapSession } from "./bootstrap";

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("bootstrapSession", () => {
  it("exchanges a valid fragment once and removes it from browser history", async () => {
    const replaceState = vi.fn();
    const bootstrap = "b".repeat(43);
    const csrfToken = "c".repeat(43);
    const fetcher = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ csrfToken }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));

    const state = await bootstrapSession(
      { hash: `#bootstrap=${bootstrap}`, pathname: "/", search: "" },
      { replaceState },
      fetcher,
    );

    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher).toHaveBeenCalledWith("/api/v1/session/bootstrap", expect.objectContaining({
      method: "POST",
      credentials: "same-origin",
      headers: expect.objectContaining({ "X-SSHC-Bootstrap": bootstrap }),
    }));
    expect(replaceState).toHaveBeenCalledWith(null, "", "/");
    expect(state.csrfToken).toBe(csrfToken);
    expect(window.sessionStorage.getItem("sshc.session.csrf")).toBe(csrfToken);
  });

   it("stores the browser enrolment returned by a one-time bootstrap", async () => {
    const browserToken = "r".repeat(43);
    const fetcher = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ csrfToken: "c".repeat(43), browserToken }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));
    await bootstrapSession(
      { hash: `#bootstrap=${"b".repeat(43)}`, pathname: "/", search: "" },
      { replaceState: vi.fn() },
      fetcher,
    );
    expect(window.localStorage.getItem("sshc.browser.registration.v1")).toBe(browserToken);
  });

  it.each([
    ["short", "#bootstrap=short", "invalid_bootstrap_fragment"],
    ["overlong", `#bootstrap=${"b".repeat(64)}`, "invalid_bootstrap_fragment"],
  ])("rejects a %s fragment before fetch", async (_name, hash, code) => {
    const fetcher = vi.fn();

    await expect(bootstrapSession(
      { hash, pathname: "/", search: "" },
      { replaceState: vi.fn() },
      fetcher,
    )).rejects.toThrow(code);

    expect(fetcher).not.toHaveBeenCalled();
  });

  it("rejects a non-success response", async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 401 }));

    await expect(bootstrapSession(
      { hash: `#bootstrap=${"b".repeat(43)}`, pathname: "/", search: "" },
      { replaceState: vi.fn() },
      fetcher,
    )).rejects.toThrow("bootstrap_rejected");
  });

  it("rejects a malformed response without persistent storage", async () => {
    const localSet = vi.spyOn(Storage.prototype, "setItem");
    const fetcher = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ csrfToken: "short" }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));

    await expect(bootstrapSession(
      { hash: `#bootstrap=${"b".repeat(43)}`, pathname: "/", search: "" },
      { replaceState: vi.fn() },
      fetcher,
    )).rejects.toThrow("invalid_bootstrap_response");

    expect(localSet).not.toHaveBeenCalled();
    expect(window.localStorage).toHaveLength(0);
    expect(window.sessionStorage).toHaveLength(0);
  });
});

describe("a reload", () => {
  it("renews the token for the session the cookie already names", async () => {
    const replaceState = vi.fn();
    const oldToken = "c".repeat(43);
    const csrfToken = "d".repeat(43);
    window.sessionStorage.setItem("sshc.session.csrf", oldToken);
    const fetcher = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ csrfToken }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));

    const state = await bootstrapSession({ hash: "", pathname: "/", search: "" }, { replaceState }, fetcher);

    expect(fetcher).toHaveBeenCalledWith("/api/v1/session/renew", expect.objectContaining({
      method: "POST",
      credentials: "same-origin",
      headers: expect.objectContaining({ "X-SSHC-CSRF": oldToken }),
    }));
    expect(state.csrfToken).toBe(csrfToken);
    expect(window.sessionStorage.getItem("sshc.session.csrf")).toBe(csrfToken);
    expect(replaceState).not.toHaveBeenCalled();
  });

  it("does not try cookie-only renewal when this port-origin has no token", async () => {
    const fetcher = vi.fn();

    await expect(
      bootstrapSession({ hash: "", pathname: "/", search: "" }, { replaceState: vi.fn() }, fetcher),
    ).rejects.toThrow("session_expired");

    expect(fetcher).not.toHaveBeenCalled();
  });

  it("recovers a new tab from the enrolled browser without a terminal URL", async () => {
    const browserToken = "r".repeat(43);
    const csrfToken = "d".repeat(43);
    window.localStorage.setItem("sshc.browser.registration.v1", browserToken);
    const fetcher = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ csrfToken }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));

    const state = await bootstrapSession({ hash: "", pathname: "/", search: "" }, { replaceState: vi.fn() }, fetcher);

    expect(fetcher).toHaveBeenCalledWith("/api/v1/session/recover", expect.objectContaining({
      method: "POST",
      credentials: "same-origin",
      headers: { "X-SSHC-Browser": browserToken },
    }));
    expect(state.csrfToken).toBe(csrfToken);
  });

  it("falls back to browser recovery after an engine restart invalidates the cookie", async () => {
    window.sessionStorage.setItem("sshc.session.csrf", "c".repeat(43));
    window.localStorage.setItem("sshc.browser.registration.v1", "r".repeat(43));
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response("", { status: 401 }))
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ csrfToken: "d".repeat(43) }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ));

    await expect(bootstrapSession({ hash: "", pathname: "/", search: "" }, { replaceState: vi.fn() }, fetcher))
      .resolves.toEqual({ csrfToken: "d".repeat(43) });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("says the session is gone when the cookie no longer names one", async () => {
    window.sessionStorage.setItem("sshc.session.csrf", "c".repeat(43));
    const fetcher = vi.fn().mockResolvedValue(new Response("", { status: 401 }));

    await expect(
      bootstrapSession({ hash: "", pathname: "/", search: "" }, { replaceState: vi.fn() }, fetcher),
    ).rejects.toThrow("session_expired");
    expect(window.sessionStorage).toHaveLength(0);
  });
});
