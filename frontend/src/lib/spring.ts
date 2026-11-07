// A spring, the way the Mac moves what bounces: SwiftUI's spring with a
// duration and a bounce. A thing on a spring is pulled towards where it is
// to go, goes a little past it and settles, and a new place to go, given
// while it moves, takes it from where it is with the speed it has, so it
// never jumps. The dictation microphone hangs on one and is pulled to how
// loud the voice is, issue #739. Followed like a meter's needle instead,
// up fast and down slowly with no bounce, Tim found it stiff.
//
// Apple turns the two into a spring of mass 1 with a stiffness of
// (2π / duration)² and a damping of 4π (1 - bounce) / duration, see
// "Animate with springs", WWDC 2023. It calls a bounce of 0.15 subtle,
// 0.3 clearly bouncy and over 0.4 too much for an interface. The duration
// is about how long it takes to settle.

export class Spring {
  // Where it is and how fast it moves, in units a second.
  at = 0;
  speed = 0;
  private stiffness: number;
  private damping: number;

  constructor(duration: number, bounce: number) {
    this.stiffness = ((2 * Math.PI) / duration) ** 2;
    this.damping = (4 * Math.PI * (1 - bounce)) / duration;
  }

  // step moves it for ms milliseconds towards to, and says whether it has
  // come to rest there. It moves in steps of at most 4 ms, so a long frame
  // moves it as far as short ones would and never throws it off, and it
  // comes to rest once it is within a thousandth of the way and nearly
  // still, so the frames stop.
  step(to: number, ms: number): boolean {
    for (let done = 0; done < ms; done += 4) {
      const h = Math.min(4, ms - done) / 1000;
      this.speed += (this.stiffness * (to - this.at) - this.damping * this.speed) * h;
      this.at += this.speed * h;
    }
    if (Math.abs(to - this.at) > 0.001 || Math.abs(this.speed) > 0.01) return false;
    this.at = to;
    this.speed = 0;
    return true;
  }
}
