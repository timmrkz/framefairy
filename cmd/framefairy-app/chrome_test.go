package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// A measurement crosses to the interface on whole pixels and never as a
// fraction, because something sitting between two pixels is painted in one
// place normally and another the moment anything puts it on a surface of
// its own.
func TestAMeasurementIsAlwaysAWholePixel(t *testing.T) {
	cases := []struct {
		in   float64
		want int
	}{
		{0, 0}, {13, 13}, {13.4, 13}, {13.5, 14}, {13.6, 14},
		{52.499, 52}, {52.5, 53},
		// Nothing about the app's window is negative, and one that has not
		// been laid out yet can answer anything at all.
		{-1, 0}, {-0.4, 0},
	}
	for _, c := range cases {
		if got := px(c.in); got != c.want {
			t.Errorf("px(%v) was %d, wanted %d", c.in, got, c.want)
		}
	}
}

// Going in and out of fullscreen is the one case where there is nothing to
// measure: no title bar, no buttons. The bar has to keep the height it had
// or the whole workspace under it jumps twice per trip.
func TestFullscreenKeepsTheBarAndLosesTheButtons(t *testing.T) {
	c := &chromeWatch{}
	normal := chromeRaw{bar: 52, left: 20, right: 92, middle: 26}

	got := c.settle(normal)
	if (got != Chrome{Bar: 52, Left: 20, Right: 92, Middle: 26}) {
		t.Fatalf("outside fullscreen gave %+v", got)
	}

	// The event says so before the animation starts, so no frame of it
	// leaks a bar that is shrinking away.
	c.set(true)
	if got := c.settle(chromeRaw{bar: 30, left: 20, right: 92, middle: 15}); got != (Chrome{Bar: 52}) {
		t.Errorf("halfway into fullscreen gave %+v, wanted the bar it had and no buttons", got)
	}
	if got := c.settle(chromeRaw{fullscreen: true}); got != (Chrome{Bar: 52}) {
		t.Errorf("in fullscreen gave %+v, wanted the bar it had and no buttons", got)
	}
	// On the way out the flag is still set, so the bar holds until the
	// macOS says it is out.
	if got := c.settle(chromeRaw{bar: 12, left: 20, right: 92, middle: 6}); got != (Chrome{Bar: 52}) {
		t.Errorf("halfway out gave %+v, wanted the bar it had", got)
	}
	c.set(false)
	after := chromeRaw{bar: 52, left: 20, right: 92, middle: 26}
	if got := c.settle(after); got != (Chrome{Bar: 52, Left: 20, Right: 92, Middle: 26}) {
		t.Errorf("out again gave %+v", got)
	}

	// A bar that comes back a different size is remembered at the new
	// one, so the next trip holds the right height.
	bigger := chromeRaw{bar: 60, left: 24, right: 100, middle: 30}
	c.settle(bigger)
	c.set(true)
	if got := c.settle(chromeRaw{fullscreen: true}); got != (Chrome{Bar: 60}) {
		t.Errorf("the second trip gave %+v, wanted the bar it had by then", got)
	}
}

// A measurement that says nothing is not allowed to wipe the bar. The app
// answers zero before it has been laid out, and the page keeps whatever
// the stylesheet says rather than collapsing.
func TestAWindowThatCannotAnswerLeavesTheBarAlone(t *testing.T) {
	c := &chromeWatch{}
	if got := c.settle(chromeRaw{}); got != (Chrome{}) {
		t.Errorf("a measurement with nothing to say gave %+v", got)
	}
	c.settle(chromeRaw{bar: 52, left: 20, right: 92, middle: 26})
	c.set(true)
	if got := c.settle(chromeRaw{fullscreen: true}); got != (Chrome{Bar: 52}) {
		t.Errorf("a bar of zero was remembered, gave %+v", got)
	}
}

// Dragging the app by its corner raises one of these a frame and almost
// none of them change anything, so the interface is told only what is new.
func TestTheWindowIsToldOnlyWhatChanged(t *testing.T) {
	answer := chromeRaw{bar: 52, left: 20, right: 92, middle: 26}
	was := measureChrome
	measureChrome = func(*application.WebviewWindow) chromeRaw { return answer }
	defer func() { measureChrome = was }()

	c := &chromeWatch{}
	said := 0
	// publish without an app says nothing, so the counting is done here on
	// the same comparison publish makes.
	tell := func() {
		now := c.measure()
		if !c.told || c.said != now {
			said++
		}
		c.said = now
		c.told = true
	}
	for i := 0; i < 20; i++ {
		tell()
	}
	if said != 1 {
		t.Errorf("twenty measurements of an unmoved app said %d things, wanted 1", said)
	}
	answer.bar = 60
	tell()
	if said != 2 {
		t.Errorf("an app that changed said %d things, wanted 2", said)
	}
}
