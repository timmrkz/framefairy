import { describe, expect, test } from "vitest";
import { shownFolder, shownPath } from "./paths";

describe("a path the way the app shows it", () => {
  test.each([
    ["from ~", "~/.framefairy/training", "/Users/tim", "/Users/tim/.framefairy/training"],
    ["the home folder itself", "~", "/Users/tim", "/Users/tim"],
    ["a home given with a slash at its end", "~/Movies", "/Users/tim/", "/Users/tim/Movies"],
    ["already in full", "/Users/tim/.framefairy/models", "/Users/tim", "/Users/tim/.framefairy/models"],
    ["outside the home folder", "/Applications/Frame Fairy.app/Contents/MacOS/ffmpeg", "/Users/tim", "/Applications/Frame Fairy.app/Contents/MacOS/ffmpeg"],
    ["a name that only starts with ~", "~tim/Movies", "/Users/tim", "~tim/Movies"],
    ["on Windows", "~\\Videos", "C:\\Users\\tim", "C:\\Users\\tim\\Videos"],
    ["with no home said", "~/Movies", "", "~/Movies"],
    ["empty", "", "/Users/tim", ""],
  ])("%s", (_, path, home, shown) => {
    expect(shownPath(path, home)).toBe(shown);
  });
});

describe("a folder the way the app shows it", () => {
  test.each([
    ["in full, with a slash at its end", "/Users/tim/.framefairy/models", "/Users/tim", "/Users/tim/.framefairy/models/"],
    ["from ~", "~/.framefairy/models", "/Users/tim", "/Users/tim/.framefairy/models/"],
    ["with its slash already", "/Users/tim/Movies/", "/Users/tim", "/Users/tim/Movies/"],
    ["on Windows", "C:\\Users\\tim\\.framefairy\\models", "C:\\Users\\tim", "C:\\Users\\tim\\.framefairy\\models\\"],
    ["not said yet", "", "/Users/tim", ""],
  ])("%s", (_, path, home, shown) => {
    expect(shownFolder(path, home)).toBe(shown);
  });
});
