import { describe, expect, it } from "vitest";
import { createReadScope, readOptions, wasAborted } from "./readScope";

describe("createReadScope", () => {
  it("aborts the reads running when it ends, and not the ones started later", () => {
    const reads = createReadScope();
    const running = reads.signal();
    reads.end();
    expect(running.aborted).toBe(true);
    expect(reads.signal().aborted).toBe(false);
  });
});

describe("readOptions", () => {
  it("adds nothing for an engine whose reads are not ended", () => {
    expect(readOptions(undefined)).toEqual({});
  });

  it("carries the scope's signal", () => {
    const reads = createReadScope();
    expect(readOptions(reads).signal).toBe(reads.signal());
  });
});

describe("wasAborted", () => {
  it("tells an aborted request from a failed one", () => {
    expect(wasAborted(new DOMException("Aborted", "AbortError"))).toBe(true);
    expect(wasAborted(new Error("network"))).toBe(false);
  });
});
