import { describe, expect, it } from "vitest";
import { cleanTags, plainText, repoName } from "./text";
import { ago } from "./time";

describe("plainText", () => {
  it("drops Markdown markers and joins lines", () => {
    expect(plainText("# Steps\n- [x] **log** in\n1. see `/login`\n```\ncode\n```\n> [docs](https://x.y)")).toBe(
      "Steps log in see /login docs",
    );
  });
});

describe("cleanTags", () => {
  it("tidies tags the way the server stores them", () => {
    expect(cleanTags(" Bug, UI,bug ,, needs  review")).toEqual(["bug", "ui", "needs review"]);
  });
});

describe("repoName", () => {
  it("finds owner/repo in any git URL", () => {
    expect(repoName("git@github.com:you/kan-do-data.git")).toBe("you/kan-do-data");
    expect(repoName("https://github.com/you/kan-do-data")).toBe("you/kan-do-data");
  });
});

describe("ago", () => {
  const now = Date.parse("2026-10-27T12:00:00Z");
  it("says how long ago", () => {
    expect(ago("2026-10-27T11:59:30Z", now)).toBe("just now");
    expect(ago("2026-10-27T11:55:00Z", now)).toBe("5m ago");
    expect(ago("2026-10-27T09:00:00Z", now)).toBe("3h ago");
    expect(ago("2026-10-25T12:00:00Z", now)).toBe("2d ago");
  });
});
