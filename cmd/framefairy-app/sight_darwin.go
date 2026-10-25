//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/message.h>

static WKWebView *drawingWebView(NSView *view) {
	if ([view isKindOfClass:[WKWebView class]]) {
		return (WKWebView *)view;
	}
	for (NSView *sub in [view subviews]) {
		WKWebView *found = drawingWebView(sub);
		if (found != nil) {
			return found;
		}
	}
	return nil;
}

// drawingKeep tells the web view of the app's window not to count the
// window as hidden when other windows cover it. The setting is
// _windowOcclusionDetectionEnabled, a property of WKWebView in WebKit's
// own header, WKWebViewPrivate.h, not Apple's public API. A window that is
// minimised, on another Space or hidden with the app is still a hidden
// page, since WebKit asks the window itself for those.
//
// It answers 0 with no web view in the window, 1 where WebKit has no such
// setting, and 2 once it is set. Packaging should know it is here, as for
// refreshFree in refresh_darwin.go.
static int drawingKeep(void *nsWindow) {
	NSWindow *window = (NSWindow *)nsWindow;
	if (window == nil) {
		return 0;
	}
	WKWebView *view = drawingWebView([window contentView]);
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

// drawingKeep keeps the page of the app's window drawn while it is
// covered. It must be called on the main thread, which is what the caller
// sees to.
func drawingKeep(window unsafe.Pointer) int {
	if window == nil {
		return drawingNoWebView
	}
	return int(C.drawingKeep(window))
}
