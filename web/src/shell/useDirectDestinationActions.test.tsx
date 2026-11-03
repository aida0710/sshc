import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { configApi, type CreateConnectionResponse } from "../api/config";
import type { DirectDestination } from "../connections/directDestination";
import { useDirectDestinationActions } from "./useDirectDestinationActions";

const destination: DirectDestination = { text: "deploy@10.0.0.5:2222", user: "deploy", host: "10.0.0.5", port: 2222 };

function created(alias: string): CreateConnectionResponse {
  return { transactionId: "tx", identity: { path: "config", alias }, preview: { operation: "connection.create", diffs: [] } };
}

function actions(overrides: Partial<Parameters<typeof useDirectDestinationActions>[0]> = {}) {
  const open = vi.fn(async () => undefined);
  const onSaved = vi.fn();
  const { result } = renderHook(() => useDirectDestinationActions({ aliases: [], canSave: true, open, onSaved, ...overrides }));
  return { result, open, onSaved };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useDirectDestinationActions", () => {
  it("opens what was typed, without writing anything", () => {
    const createConnection = vi.spyOn(configApi, "createConnection");
    const { result, open } = actions();
    result.current.connect(destination, "w1");
    expect(open).toHaveBeenCalledExactlyOnceWith("deploy@10.0.0.5:2222", undefined, "w1");
    expect(createConnection).not.toHaveBeenCalled();
  });

  it("writes a Host block that asks for the password on connecting, and opens the connection it made", async () => {
    const createConnection = vi.spyOn(configApi, "createConnection").mockResolvedValue(created("10-0-0-5"));
    const { result, open, onSaved } = actions();
    await act(async () => {
      result.current.saveAndConnect?.(destination, "w1");
      await vi.waitFor(() => expect(open).toHaveBeenCalled());
    });
    expect(createConnection).toHaveBeenCalledExactlyOnceWith({
      alias: "10-0-0-5", hostName: "10.0.0.5", user: "deploy", port: 2222, authentication: { kind: "ask_on_connect" },
    });
    expect(onSaved).toHaveBeenCalledOnce();
    expect(open).toHaveBeenCalledExactlyOnceWith("10-0-0-5", undefined, "w1");
  });

  it("leaves the port to the config files when it was not typed", async () => {
    const createConnection = vi.spyOn(configApi, "createConnection").mockResolvedValue(created("web"));
    const { result, open } = actions();
    await act(async () => {
      result.current.saveAndConnect?.({ text: "deploy@web.example.com", user: "deploy", host: "web.example.com" });
      await vi.waitFor(() => expect(open).toHaveBeenCalled());
    });
    expect(createConnection).toHaveBeenCalledWith(expect.not.objectContaining({ port: expect.anything() }));
  });

  it("names it apart from a connection that already has the name", () => {
    const { result } = actions({ aliases: ["10-0-0-5"] });
    expect(result.current.aliasFor(destination)).toBe("10-0-0-5-2");
  });

  it("still gives a name to a host name that suggests none", () => {
    const { result } = actions();
    expect(result.current.aliasFor({ text: "deploy@::", user: "deploy", host: "::" })).toBe("host");
  });

  it("opens nothing when the connection could not be saved", async () => {
    const createConnection = vi.spyOn(configApi, "createConnection").mockRejectedValue(new Error("refused"));
    const { result, open, onSaved } = actions();
    await act(async () => {
      result.current.saveAndConnect?.(destination);
      await vi.waitFor(() => expect(createConnection).toHaveBeenCalled());
    });
    expect(open).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("does not offer to save where the connection could not be opened", () => {
    const { result } = actions({ canSave: false });
    expect(result.current.saveAndConnect).toBeUndefined();
  });
});
