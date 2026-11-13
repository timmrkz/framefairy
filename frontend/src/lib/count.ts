// The count of clips a window of this length suggests while a hand drags
// it, read off the lengths Go sends at which the count rises, see
// engine.SuggestedSteps and WindowView.steps. Go answers with the same
// count once the hand lets go. A length on a step counts it, so a window
// dragged to exactly the length a count rises at already suggests it.
// In a module of its own, with nothing it imports, so the walks read the
// app's own and keep no copy of it.
export function suggestedCount(steps: readonly number[], length: number): number {
  return 1 + steps.filter((at) => at <= length).length;
}
