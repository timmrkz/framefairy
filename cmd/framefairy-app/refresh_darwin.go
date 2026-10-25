//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/message.h>

struct refreshOut {
	// What came of it: 0 no web view in the window, 1 WebKit has no such
	// setting, 2 the cap was lifted.
	int done;
	// The most frames a second the window's screen can show.
	long most;
};

// webViewIn finds the web view of the app's window, for both of WebKit's
// own settings in this file.
static WKWebView *webViewIn(NSView *view) {
	if ([view isKindOfClass:[WKWebView class]]) {
		return (WKWebView *)view;
	}
	for (NSView *sub in [view subviews]) {
		WKWebView *found = webViewIn(sub);
		if (found != nil) {
			return found;
		}
	}
	return nil;
}

// refreshFree lets the page of the app's window update as often as the
// screen refreshes, where WebKit holds every page to about 60 times a
// second unless told otherwise. The setting is "Prefer Page Rendering
// Updates near 60fps" in Safari's feature flags, a feature of WebKit's
// own, reached through WKPreferences' _features and _setEnabled:
// forFeature:, which Safari's settings use.
//
// WebKit reads the setting when the page's screen changes, so the window
// is then said to have changed screen, the way macOS says it when the
// window is dragged to another. It is said a turn of the main loop later,
// once the setting has gone to the page. Everything else that listens for
// a change of screen hears it too, on every load of the page, with the
// screen as it was: Wails sends WindowDidChangeScreen, and watchChrome in
// chrome.go measures the title bar again, which changes nothing.
//
// _features and _setEnabled:forFeature: are WebKit's own, not Apple's
// public API. Where a WebKit has them no more, the hold stays and the log
// says so, see refreshSaid. Packaging should know they are here.
static struct refreshOut refreshFree(void *nsWindow) {
	struct refreshOut out = {0, 0};
	NSWindow *window = (NSWindow *)nsWindow;
	if (window == nil) {
		return out;
	}
	out.most = (long)[[window screen] maximumFramesPerSecond];
	WKWebView *view = webViewIn([window contentView]);
	if (view == nil) {
		return out;
	}
	out.done = 1;
	WKPreferences *prefs = [[view configuration] preferences];
	SEL list = NSSelectorFromString(@"_features");
	SEL set = NSSelectorFromString(@"_setEnabled:forFeature:");
	if (![WKPreferences respondsToSelector:list] || ![prefs respondsToSelector:set]) {
		return out;
	}
	NSArray *features = ((NSArray * (*)(id, SEL))objc_msgSend)([WKPreferences class], list);
	for (id feature in features) {
		if (![[feature valueForKey:@"key"] isEqual:@"PreferPageRenderingUpdatesNear60FPSEnabled"]) {
			continue;
		}
		((void (*)(id, SEL, BOOL, id))objc_msgSend)(prefs, set, NO, feature);
		dispatch_async(dispatch_get_main_queue(), ^{
			[[NSNotificationCenter defaultCenter] postNotificationName:NSWindowDidChangeScreenNotification object:window];
		});
		out.done = 2;
		return out;
	}
	return out;
}

// drawingKeep tells the web view of the app's window not to count the
// window as hidden when other windows cover it, see keepDrawing in
// sight.go. The setting is _windowOcclusionDetectionEnabled, a property of
// WKWebView in WebKit's own header, WKWebViewPrivate.h, not Apple's public
// API. WebKit then asks only whether the window is on screen at all, so a
// minimised or hidden app is still a hidden page, and one on another Space
// is not.
//
// It answers 0 with no web view in the window, 1 where WebKit has no such
// setting, and 2 once it is set.
static int drawingKeep(void *nsWindow) {
	NSWindow *window = (NSWindow *)nsWindow;
	if (window == nil) {
		return 0;
	}
	WKWebView *view = webViewIn([window contentView]);
	if (view == nil) {
		return 0;
	}
	SEL set = NSSelectorFromString(@"_setWindowOcclusionDetectionEnabled:");
	if (![view respondsToSelector:set]) {
		return 1;
	}
	((void (*)(id, SEL, BOOL))objc_msgSend)(view, set, NO);
	return 2;
}
*/
import "C"

import "unsafe"

// refreshFree lets the app's window update at the screen's own rate. It
// must be called on the main thread, which is what the caller sees to.
func refreshFree(window unsafe.Pointer) refreshRaw {
	if window == nil {
		return refreshRaw{}
	}
	out := C.refreshFree(window)
	return refreshRaw{done: int(out.done), most: int(out.most)}
}

// drawingKeep keeps the page of the app's window drawn while it is
// covered. It must be called on the main thread, which is what the caller
// sees to.
func drawingKeep(window unsafe.Pointer) int {
	if window == nil {
		return drawingNoWebView
	}
	return int(C.drawingKeep(window))
}
