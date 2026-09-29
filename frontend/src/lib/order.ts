// How the sidebar lists the episodes. The library keeps them in the order
// they were added, the newest last, so that order is the list as it comes.
export type EpisodeOrder = "added" | "name";

// Names compared the way Finder compares them: Folge 2 before Folge 10,
// and a capital no different from a small letter.
const byName = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

export function sortEpisodes<T extends { name: string }>(episodes: T[], order: EpisodeOrder): T[] {
  if (order === "added") return episodes;
  return [...episodes].sort((a, b) => byName.compare(a.name, b.name));
}
