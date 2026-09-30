import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSectionHandoffs } from "./useSectionHandoffs";

describe("useSectionHandoffs", () => {
  it("sends a config line to Config and forgets it once Config has taken it", () => {
    const navigate = vi.fn();
    const { result } = renderHook(() => useSectionHandoffs(navigate));

    act(() => result.current.openFile("conf.d/10-home.conf", 2));
    expect(navigate).toHaveBeenCalledWith("Config");
    const target = result.current.fileTarget;
    expect(target).toMatchObject({ path: "conf.d/10-home.conf", line: 2 });

    act(() => result.current.consumeFileTarget(target?.request ?? -1));
    expect(result.current.fileTarget).toBeNull();
  });

  it("keeps a newer config line when an older one is reported as taken", () => {
    const { result } = renderHook(() => useSectionHandoffs(vi.fn()));

    act(() => result.current.openFile("config", 1));
    const older = result.current.fileTarget?.request ?? -1;
    act(() => result.current.openFile("conf.d/10-home.conf", 5));
    act(() => result.current.consumeFileTarget(older));

    expect(result.current.fileTarget).toMatchObject({ path: "conf.d/10-home.conf", line: 5 });
  });

  it("forgets a remote path once Files has taken it", () => {
    const navigate = vi.fn();
    const { result } = renderHook(() => useSectionHandoffs(navigate));

    act(() => result.current.openRemotePath("edge", "/var/log/app.log", "browse"));
    expect(navigate).toHaveBeenCalledWith("Files");
    const target = result.current.sftpTarget;
    expect(target).toMatchObject({ alias: "edge", path: "/var/log/app.log", action: "browse" });

    act(() => result.current.consumeSftpTarget(target?.request ?? -1));
    expect(result.current.sftpTarget).toBeNull();
  });
});
