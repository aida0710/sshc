import { describe, expect, it, vi } from "vitest";
import { sftpPlaces, sftpPlacesStorageKey } from "./places";

// The book is a module singleton shared by every SFTP panel, so each test
// works on its own host rather than trying to rewind the shared state.
describe("SFTP places", () => {
  it("keeps bookmarks and recent paths apart and persists them", () => {
    sftpPlaces.toggleBookmark("kept", "/srv/app");
    sftpPlaces.remember("kept", "/var/log");
    sftpPlaces.remember("kept", "/home/edge");

    expect(sftpPlaces.bookmarks("kept")).toEqual(["/srv/app"]);
    expect(sftpPlaces.bookmarked("kept", "/srv/app")).toBe(true);
    expect(sftpPlaces.recent("kept")).toEqual(["/home/edge", "/var/log"]);
    expect(sftpPlaces.recent("untouched")).toEqual([]);

    const stored: unknown = JSON.parse(window.localStorage.getItem(sftpPlacesStorageKey) ?? "{}");
    expect(stored).toMatchObject({ kept: { bookmarks: ["/srv/app"], recent: ["/home/edge", "/var/log"] } });
  });

  it("moves a revisited path to the front instead of repeating it", () => {
    sftpPlaces.remember("revisit", "/one");
    sftpPlaces.remember("revisit", "/two");
    sftpPlaces.remember("revisit", "/one");

    expect(sftpPlaces.recent("revisit")).toEqual(["/one", "/two"]);
  });

  it("toggles a bookmark off and refuses paths that are not absolute", () => {
    sftpPlaces.toggleBookmark("toggled", "/srv/app");
    sftpPlaces.toggleBookmark("toggled", "/srv/app");
    sftpPlaces.toggleBookmark("toggled", "relative/path");
    sftpPlaces.toggleBookmark("", "/srv/app");

    expect(sftpPlaces.bookmarks("toggled")).toEqual([]);
    expect(sftpPlaces.bookmarks("")).toEqual([]);
  });

  it("notifies subscribers and keeps the place when storage refuses to write", () => {
    const listener = vi.fn();
    const unsubscribe = sftpPlaces.subscribe(listener);
    const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });

    sftpPlaces.toggleBookmark("quota", "/full");

    expect(listener).toHaveBeenCalled();
    expect(sftpPlaces.bookmarks("quota")).toEqual(["/full"]);
    setItem.mockRestore();
    unsubscribe();
  });
});
