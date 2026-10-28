import { describe, expect, it } from "vitest";
import { ordersAfterMove } from "./groupOrder";

describe("moving a group among its siblings", () => {
  it("numbers every sibling in its new place", () => {
    expect([...ordersAfterMove(["home", "lab", "work"], "lab", "up")!]).toEqual([["lab", 0], ["home", 1], ["work", 2]]);
  });

  it("does not move the first group up or the last group down", () => {
    expect(ordersAfterMove(["home", "lab"], "home", "up")).toBeNull();
    expect(ordersAfterMove(["home", "lab"], "lab", "down")).toBeNull();
  });
});
