import { describe, expect, it } from "vitest";
import { isReleasePageURL } from "./sshcRepository";

describe("isReleasePageURL", () => {
  it.each([
    "https://github.com/aida0710/sshc/releases/tag/v1.2.0",
    "https://github.com/aida0710/sshc/releases/latest",
    "https://GitHub.com:443/aida0710/sshc/releases/tag/v1.2.0",
  ])("accepts sshc's release page %s", (pageURL) => {
    expect(isReleasePageURL(pageURL)).toBe(true);
  });

  it.each([
    ["another host", "https://example.invalid/aida0710/sshc/releases/tag/v1.2.0"],
    ["a host that only starts like GitHub", "https://github.com.example.invalid/aida0710/sshc/releases/tag/v1.2.0"],
    ["GitHub as the user name of another host", "https://github.com@example.invalid/aida0710/sshc/releases/tag/v1.2.0"],
    ["a user name before GitHub", "https://mirage@github.com/aida0710/sshc/releases/tag/v1.2.0"],
    ["another port", "https://github.com:8443/aida0710/sshc/releases/tag/v1.2.0"],
    ["another repository", "https://github.com/pelican/sshc/releases/tag/v1.2.0"],
    ["a repository that only starts with sshc", "https://github.com/aida0710/sshc-dunlin/releases/tag/v1.2.0"],
    ["a path that only starts with releases", "https://github.com/aida0710/sshc/releases-dunlin/tag/v1.2.0"],
    ["the repository outside its releases", "https://github.com/aida0710/sshc/issues/1"],
    ["dot segments that leave the releases", "https://github.com/aida0710/sshc/releases/../../../pelican/sshc/releases/tag/v1.2.0"],
    ["one slash after the scheme", "https:/github.com/aida0710/sshc/releases/tag/v1.2.0"],
    ["no slash after the scheme", "https:github.com/aida0710/sshc/releases/tag/v1.2.0"],
    ["http", "http://github.com/aida0710/sshc/releases/tag/v1.2.0"],
    ["the sshc engine on this machine", "http://127.0.0.1:54447/api/v1/update"],
    ["text that is not a URL", "aida0710/sshc/releases/tag/v1.2.0"],
    ["no page", undefined],
  ])("rejects %s", (_case, pageURL) => {
    expect(isReleasePageURL(pageURL)).toBe(false);
  });
});
