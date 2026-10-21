//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework CoreGraphics

#import <Cocoa/Cocoa.h>
#import <CoreGraphics/CoreGraphics.h>
#include <math.h>

#define MATCH_MOST 32

struct matchModes {
	// The window's screen, 0 when the window has none.
	CGDirectDisplayID display;
	// The rates the screen offers at the size it runs at now, in Hz, and
	// which of them it runs at, or -1.
	double hz[MATCH_MOST];
	int count;
	int now;
};

static CGDirectDisplayID matchDisplay(void *nsWindow) {
	NSScreen *screen = [(NSWindow *)nsWindow screen];
	if (screen == nil) {
		return 0;
	}
	return [[[screen deviceDescription] objectForKey:@"NSScreenNumber"] unsignedIntValue];
}

// Every mode of the screen. Low resolution twins are asked for too, so
// the modes of the size the screen runs at are all there.
static CFArrayRef matchList(CGDirectDisplayID display) {
	CFStringRef keys[] = {kCGDisplayShowDuplicateLowResolutionModes};
	CFBooleanRef values[] = {kCFBooleanTrue};
	CFDictionaryRef options = CFDictionaryCreate(NULL, (const void **)keys, (const void **)values, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFArrayRef all = CGDisplayCopyAllDisplayModes(display, options);
	CFRelease(options);
	return all;
}

static int matchSame(CGDisplayModeRef a, CGDisplayModeRef b) {
	return CGDisplayModeGetWidth(a) == CGDisplayModeGetWidth(b) && CGDisplayModeGetHeight(a) == CGDisplayModeGetHeight(b) && CGDisplayModeGetPixelWidth(a) == CGDisplayModeGetPixelWidth(b) && CGDisplayModeGetPixelHeight(a) == CGDisplayModeGetPixelHeight(b) && CGDisplayModeIsUsableForDesktopGUI(a);
}

// The rate of a mode, or the most frames a second the screen can show for
// one that does not say, which is what a screen with a rate of its own
// answers.
static double matchHz(CGDisplayModeRef mode, void *nsWindow) {
	double hz = CGDisplayModeGetRefreshRate(mode);
	if (hz > 0) {
		return hz;
	}
	return (double)[[(NSWindow *)nsWindow screen] maximumFramesPerSecond];
}

static struct matchModes matchModesOf(void *nsWindow) {
	struct matchModes out = {0, {0}, 0, -1};
	out.display = matchDisplay(nsWindow);
	if (out.display == 0) {
		return out;
	}
	CGDisplayModeRef now = CGDisplayCopyDisplayMode(out.display);
	if (now == NULL) {
		out.display = 0;
		return out;
	}
	double nowHz = matchHz(now, nsWindow);
	out.hz[0] = nowHz;
	out.now = 0;
	out.count = 1;
	CFArrayRef all = matchList(out.display);
	for (CFIndex i = 0; all != NULL && i < CFArrayGetCount(all) && out.count < MATCH_MOST; i++) {
		CGDisplayModeRef m = (CGDisplayModeRef)CFArrayGetValueAtIndex(all, i);
		if (!matchSame(m, now)) {
			continue;
		}
		double hz = CGDisplayModeGetRefreshRate(m);
		int seen = 0;
		for (int k = 0; k < out.count; k++) {
			if (fabs(out.hz[k] - hz) < 0.005) {
				seen = 1;
			}
		}
		if (hz > 0 && !seen) {
			out.hz[out.count++] = hz;
		}
	}
	if (all != NULL) {
		CFRelease(all);
	}
	CGDisplayModeRelease(now);
	return out;
}

// Whether the screen was changed for the app, so there is something to
// give back.
static int matchChanged = 0;

// The window was said to have changed screen, the moment WebKit reads the
// screen's rate again, see refreshFree.
static void matchTell(void *nsWindow) {
	NSWindow *window = (NSWindow *)nsWindow;
	dispatch_async(dispatch_get_main_queue(), ^{
		[[NSNotificationCenter defaultCenter] postNotificationName:NSWindowDidChangeScreenNotification object:window];
	});
}

// Runs the screen at hz, for as long as the app runs. kCGConfigureForAppOnly
// is what makes macOS give the screen its own mode back when the app quits,
// crashed or not. 1 when it took.
static int matchSet(void *nsWindow, CGDirectDisplayID display, double hz) {
	CGDisplayModeRef now = CGDisplayCopyDisplayMode(display);
	if (now == NULL) {
		return 0;
	}
	CGDisplayModeRef pick = NULL;
	CFArrayRef all = matchList(display);
	for (CFIndex i = 0; all != NULL && i < CFArrayGetCount(all); i++) {
		CGDisplayModeRef m = (CGDisplayModeRef)CFArrayGetValueAtIndex(all, i);
		if (matchSame(m, now) && fabs(CGDisplayModeGetRefreshRate(m) - hz) < 0.005) {
			pick = m;
			break;
		}
	}
	int ok = 0;
	CGDisplayConfigRef config;
	if (pick != NULL && CGBeginDisplayConfiguration(&config) == kCGErrorSuccess) {
		if (CGConfigureDisplayWithDisplayMode(config, display, pick, NULL) == kCGErrorSuccess && CGCompleteDisplayConfiguration(config, kCGConfigureForAppOnly) == kCGErrorSuccess) {
			ok = 1;
			matchChanged = 1;
			matchTell(nsWindow);
		} else {
			CGCancelDisplayConfiguration(config);
		}
	}
	if (all != NULL) {
		CFRelease(all);
	}
	CGDisplayModeRelease(now);
	return ok;
}

// Gives every screen back the mode it had before the app changed it. 1
// when there was something to give back.
static int matchGiveBack(void *nsWindow) {
	if (!matchChanged) {
		return 0;
	}
	matchChanged = 0;
	CGRestorePermanentDisplayConfiguration();
	matchTell(nsWindow);
	return 1;
}
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type matchAnswer struct {
	done     int
	from, to float64
	modes    []screenMode
}

// screenMatch runs the window's screen at a rate that shows a video of
// fps frames a second evenly, see matchRate, or gives the screen back for
// fps 0. AppKit and the screen's modes are reached on the main thread.
func screenMatch(window unsafe.Pointer, fps float64) (int, float64, float64, []screenMode) {
	if window == nil {
		return matchNoScreen, 0, 0, nil
	}
	a := application.InvokeSyncWithResult(func() matchAnswer {
		listed := C.matchModesOf(window)
		var a matchAnswer
		for i := 0; i < int(listed.count); i++ {
			a.modes = append(a.modes, screenMode{hz: float64(listed.hz[i]), now: i == int(listed.now)})
		}
		if len(a.modes) > 0 {
			a.from = a.modes[0].hz
		}
		if fps <= 0 {
			a.done = matchNothingToGive
			if C.matchGiveBack(window) == 1 {
				a.done = matchGivenBack
			}
			return a
		}
		if listed.display == 0 {
			a.done = matchNoScreen
			return a
		}
		hz := matchRate(a.modes, fps)
		switch {
		case hz == 0 && evenOn(a.from, fps):
			a.done = matchEven
		case hz == 0:
			a.done = matchNone
		case C.matchSet(window, listed.display, C.double(hz)) == 1:
			a.done, a.to = matchSwitched, hz
		default:
			a.done, a.to = matchFailed, hz
		}
		return a
	})
	return a.done, a.from, a.to, a.modes
}
