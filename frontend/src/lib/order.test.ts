import { describe, expect, it } from "vitest";
import { sortEpisodes } from "./order";

const eps = ["start.mp4", "youtube.mp4", "end.mp4", "Folge 10.mp4", "folge 2.mp4"].map((name) => ({ name }));
const names = (list: { name: string }[]) => list.map((e) => e.name);

describe("sortEpisodes", () => {
  it("keeps the order the episodes were added in", () => {
    expect(names(sortEpisodes(eps, "added"))).toEqual(names(eps));
  });

  it("sorts by name the way Finder does, numbers as numbers and no case", () => {
    expect(names(sortEpisodes(eps, "name"))).toEqual(["end.mp4", "folge 2.mp4", "Folge 10.mp4", "start.mp4", "youtube.mp4"]);
  });

  it("leaves the library's own list alone", () => {
    const before = names(eps);
    sortEpisodes(eps, "name");
    expect(names(eps)).toEqual(before);
  });
});
