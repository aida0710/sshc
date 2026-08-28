import { describe, expect, it } from "vitest";
import { removeHostBlock } from "./blocks";

const contents = "# top\nHost bastion\n\tUser ops\n\nHost nas\n\tUser aida\n";

describe("removeHostBlock", () => {
  it("removes exactly the block that starts at the given line", () => {
    expect(removeHostBlock(contents, 2, "Host bastion\n\tUser ops\n\n")).toBe("# top\nHost nas\n\tUser aida\n");
  });

  it("refuses to remove when the text at that line is not the block", () => {
    expect(() => removeHostBlock(contents, 2, "Host other\n")).toThrow("block_moved");
  });
});

describe("a block's attached comment", () => {
  const contents =
    "# the production bastion\n" +
    "# ask infra first\n" +
    "Host bastion\n" +
    "\tPort 2222\n" +
    "\n" +
    "Host nas\n" +
    "\tPort 22\n";

  it("is removed with the block it belongs to", () => {
    const raw = "Host bastion\n\tPort 2222\n";
    const after = removeHostBlock(contents, 3, raw, 2);

    expect(after).toBe("\nHost nas\n\tPort 22\n");
    expect(after).not.toContain("the production bastion");
  });

  it("is left alone when the block has none", () => {
    const plain = "Host bastion\n\tPort 2222\nHost nas\n";
    expect(removeHostBlock(plain, 1, "Host bastion\n\tPort 2222\n", 0)).toBe("Host nas\n");
  });

  it("removes a block that starts the file together with its comment", () => {
    const after = removeHostBlock("# only\nHost nas\n", 2, "Host nas\n", 1);
    expect(after).toBe("");
  });
});
