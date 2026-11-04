// A video's session in the workspace, issue #606. It keeps the video's
// state as Go sends it, the newest there is, and says when the app chose
// another clip, once. Everything the video's work changes comes in that
// state, the clips a search wrote, how far the video is heard, what a
// render made, so nothing waits for the work to move on and reads the
// video again. See
// docs/decisions/2026-10-30-a-videos-state-in-one-session.md and
// docs/decisions/2026-11-01-every-rule-of-the-workspace-in-go.md.
import type { ClipEntry, CoverageView, EpisodeStatus, Job, KeptWindow, VideoState, WindowView } from "./api";
import type { JobStore } from "./state.svelte";
import { unchanged } from "./unchanged";

// What the workspace does when the app chose another clip, see
// cmd/framefairy-app/choose.go, in place of before, the clip chosen until
// then. card is the card the clip was on its way as, when it was chosen
// because that card was written as it, or nothing.
export type Chose = (key: string, before: string, card: string) => void;

export class EpisodeSession {
  // The video's state, from the newest VideoState kept: null until the
  // first is in.
  status = $state<EpisodeStatus | null>(null);
  clips = $state<ClipEntry[]>([]);
  coverage = $state<CoverageView>({ passes: [] });
  // The rev of the newest state kept.
  rev = $state(0);
  // The video's jobs in the newest state kept, the clips on their way
  // among them. They come with the clips, so a card on its way becomes its
  // clip in the state that holds the clip, never a frame before or after.
  jobs = $state<Job[]>([]);
  // The rev of the newest answer to an edit: a state read before it
  // knows nothing of the edit, and would put back what it changed.
  private floor = 0;
  // Changes shown before the Go side has answered them, and the newest
  // state that came in the meantime, kept until they are answered.
  private holds = 0;
  private waiting: VideoState | null = null;
  private waitingFresh = false;
  // The clip chosen, by a hand or by the app, and the video's window, from
  // the newest state kept, or what a hand did while the Go side has not
  // answered it, see Hand.
  chosen = $state("");
  window = $state<WindowView | null>(null);
  private picks = new Hand();
  private places = new Hand();
  private chose: Chose = () => {};

  constructor(
    readonly path: string,
    private store: JobStore,
  ) {}

  // The workspace is on the page: from now on the session says when the
  // app chose another clip with chose, and takes every state of the video
  // hear brings, onVideoState in lib/api.ts. The function it returns is
  // for when the workspace is gone, and then it hears nothing more.
  open(chose: Chose, hear: (fn: (state: VideoState) => void) => () => void = () => () => {}): () => void {
    this.chose = chose;
    return hear((state) => {
      if (state.video === this.path) this.take(state);
    });
  }

  // A state of the video, sent by the Go side or read by the workspace.
  // It is kept when it is newer than every state and every edit's answer
  // kept before it, and fresh makes every clip in it a new one, so the
  // captions of the clip on screen are read again after an edit of them,
  // the clips kept when the state is older than them.
  // A state sent as work goes on keeps each clip it holds unchanged the
  // very same clip, so nothing about the clip on screen is drawn again.
  // Its jobs go to the store whether it is kept or not, those the store
  // has no newer snapshot of, so the jobs on screen are never older than
  // the state. Whether it was kept.
  take(state: VideoState, fresh = false): boolean {
    let kept = false;
    if (this.holds > 0) {
      if (!this.waiting || state.rev > this.waiting.rev) this.waiting = state;
      this.waitingFresh ||= fresh;
    } else if (state.rev > this.rev && state.rev > this.floor) {
      this.rev = state.rev;
      this.status = same(this.status, state.status);
      this.clips = fresh ? state.clips : unchanged(this.clips, state.clips);
      this.coverage = same(this.coverage, state.coverage);
      this.jobs = state.jobs;
      kept = true;
      this.choose(state);
      this.frame(state);
    } else if (fresh) {
      // A newer state came first, and still the captions are read again.
      this.clips = this.clips.map((c) => ({ ...c }));
    }
    this.store.take(state);
    return kept;
  }

  // The clip a state says is chosen, once no pick is on its way that the
  // state was read before. A state that says the card picked was written
  // as its clip is read after the pick, whenever its answer comes: the Go
  // side had the pick, the card is gone from the state's jobs and its clip
  // is in its clips, so the clip is chosen with them. Held for the pick's
  // answer, the card was gone and nothing chosen for as long as the answer
  // took on a busy machine, issue #697.
  private choose(state: VideoState) {
    const written = !!state.chosenCard && state.chosenCard === this.chosen;
    if (this.picks.holds(state) && !written) return;
    const now = state.chosen ?? "";
    if (now === this.chosen) return;
    const before = this.chosen;
    this.chosen = now;
    this.chose(now, before, state.chosenCard ?? "");
  }

  // The window a state says, once no move of a hand is on its way that the
  // state was read before.
  private frame(state: VideoState) {
    if (this.places.holds(state)) return;
    this.window = same(this.window, state.window ?? null);
  }

  // A clip chosen by a hand, shown at once while choose tells the Go side,
  // which answers with the state of the video read after the pick. What
  // that state says is chosen is drawn, which is the clip picked, or the
  // clip a card picked was written as. A pick that is refused leaves the
  // clip chosen to the next state. What it returns is done once the
  // answer is in.
  pick(key: string, choose: (key: string) => Promise<VideoState>): Promise<void> {
    this.chosen = key;
    return this.picks.ask(
      () => choose(key),
      (state) => this.take(state),
      (late) => this.choose(late),
    );
  }

  // The window as a hand put it, shown at once while place tells the Go
  // side, which answers with the state of the video read after it, whose
  // window it fitted to what a search can do with it.
  place(w: KeptWindow, place: (w: KeptWindow) => Promise<VideoState>): Promise<void> {
    if (this.window) this.window = { ...this.window, ...w };
    return this.places.ask(
      () => place(w),
      (state) => this.take(state),
      (late) => this.frame(late),
    );
  }

  // One clip as an edit left it, from the edit's answer. No state read
  // before the edit is kept after it.
  put(clip: ClipEntry) {
    this.floor = Math.max(this.floor, clip.rev ?? 0);
    this.clips = this.clips.map((c) => (c.key === clip.key ? clip : c));
  }

  // One clip changed on screen at once, the way a hand left it, while
  // write saves it. Its answer takes the change's place, and a write that
  // is refused puts the clip back as it was and throws. The states that
  // come while it writes are kept aside, because each was read before the
  // edit and would undo it on screen for a moment. The newest of them is
  // taken once it has answered, unless the answer is newer.
  async edit(key: string, to: (clip: ClipEntry) => ClipEntry, write: () => Promise<ClipEntry>): Promise<void> {
    const was = this.clips.find((c) => c.key === key);
    this.holds++;
    this.clips = this.clips.map((c) => (c.key === key ? to(c) : c));
    try {
      this.put(await write());
    } catch (err) {
      if (was) this.clips = this.clips.map((c) => (c.key === key ? was : c));
      throw err;
    } finally {
      this.holds--;
      const next = this.waiting;
      if (this.holds === 0 && next) {
        const fresh = this.waitingFresh;
        this.waiting = null;
        this.waitingFresh = false;
        this.take(next, fresh);
      }
    }
  }
}

// The value kept, when the one that came says the same, so nothing that
// reads it runs again for a state that changed elsewhere.
function same<T>(was: T, now: T): T {
  return was !== null && JSON.stringify(was) === JSON.stringify(now) ? was : now;
}

// What a hand did that the Go side has not answered yet: a pick, or a
// window put somewhere. It is shown at once, and the Go side answers with
// the state of the video read after it. A state read before that answer
// may know nothing of what the hand did and would put back what was there
// for a moment, so asking counts the calls not answered and at is the rev
// of the newest answer. The newest state that came while one was on its
// way, the answer among them, is kept, so what it says is shown once
// every call is answered.
class Hand {
  private asking = 0;
  private at = 0;
  private late: VideoState | null = null;

  // Whether what a state says is held back: kept for later while a call
  // is on its way, or older than the newest answer.
  holds(state: VideoState): boolean {
    if (this.asking > 0) {
      if (!this.late || state.rev > this.late.rev) this.late = state;
      return true;
    }
    return state.rev < this.at;
  }

  // call is answered with a state, which take keeps. A call that is
  // refused leaves what is shown to the next state. show is given the
  // newest state that came meanwhile, once nothing is on its way.
  ask(call: () => Promise<VideoState>, take: (state: VideoState) => void, show: (late: VideoState) => void): Promise<void> {
    this.asking++;
    return call()
      .then((state) => {
        this.at = Math.max(this.at, state.rev);
        take(state);
      })
      .catch(() => {
        this.at = 0;
      })
      .finally(() => {
        this.asking--;
        const late = this.late;
        if (this.asking === 0 && late) {
          this.late = null;
          show(late);
        }
      });
  }
}
