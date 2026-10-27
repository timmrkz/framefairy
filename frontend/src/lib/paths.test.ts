import { describe, expect, test } from "vitest";
import { shownPath } from "./paths";

describe("a path the way the app shows it", () => {
  test.each([
    ["inside the home folder", "/Users/tim/.framefairy/models", "/Users/tim", "~/.framefairy/models"],
    ["the home folder itself", "/Users/tim", "/Users/tim", "~"],
    ["a home given with a slash at its end", "/Users/tim/Movies", "/Users/tim/", "~/Movies"],
    ["already from ~", "~/.framefairy/training", "/Users/tim", "~/.framefairy/training"],
    ["outside the home folder", "/Applications/Frame Fairy.app/Contents/MacOS/ffmpeg", "/Users/tim", "/Applications/Frame Fairy.app/Contents/MacOS/ffmpeg"],
    ["a folder whose name only starts like the home", "/Users/timothy/Movies", "/Users/tim", "/Users/timothy/Movies"],
    ["on Windows", "C:\\Users\\tim\\Videos", "C:\\Users\\tim", "~\\Videos"],
    ["with no home said", "/Users/tim/Movies", "", "/Users/tim/Movies"],
    ["empty", "", "/Users/tim", ""],
  ])("%s", (_, path, home, shown) => {
    expect(shownPath(path, home)).toBe(shown);
  });
});
