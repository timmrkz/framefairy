//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

struct chromeOut {
	double bar;
	double left;
	double right;
	double middle;
	bool fullscreen;
};

// chromeMeasure asks the window where macOS put its own furniture.
//
// Everything is worked out in the content view's own space, which is the
// page, because the window is asked for a title bar the page reaches
// under. AppKit counts upwards from the bottom, so a distance from the top
// is the top of that space less whatever is being asked about.
//
// contentLayoutRect is the part of the content area the title bar and the
// toolbar do not cover, so the height of the bar is what is left above it.
// It is given in the window's space, hence the conversion from nil.
//
// It answers zeros where there is nothing to ask, and the app leaves the
// bar as the stylesheet has it.
static struct chromeOut chromeMeasure(void *nsWindow) {
	struct chromeOut out = {0, 0, 0, 0, false};
	NSWindow *window = (NSWindow *)nsWindow;
	if (window == nil) {
		return out;
	}
	NSView *content = [window contentView];
	if (content == nil) {
		return out;
	}
	out.fullscreen = ([window styleMask] & NSWindowStyleMaskFullScreen) != 0;

	NSRect bounds = [content bounds];
	NSRect layout = [content convertRect:[window contentLayoutRect] fromView:nil];
	out.bar = NSMaxY(bounds) - NSMaxY(layout);
	if (out.bar < 0) {
		out.bar = 0;
	}

	NSButton *close = [window standardWindowButton:NSWindowCloseButton];
	NSButton *zoom = [window standardWindowButton:NSWindowZoomButton];
	if (close == nil || zoom == nil || [close isHidden] || [zoom isHidden]) {
		return out;
	}
	NSRect c = [close convertRect:[close bounds] toView:content];
	NSRect z = [zoom convertRect:[zoom bounds] toView:content];
	out.left = NSMinX(c);
	out.right = NSMaxX(z);
	out.middle = NSMaxY(bounds) - NSMidY(c);
	return out;
}
*/
import "C"

import "unsafe"

// chromeMeasure asks macOS about the window. It must be called on the main
// thread, which is what the caller sees to.
func chromeMeasure(window unsafe.Pointer) chromeRaw {
	if window == nil {
		return chromeRaw{}
	}
	out := C.chromeMeasure(window)
	return chromeRaw{
		bar:        float64(out.bar),
		left:       float64(out.left),
		right:      float64(out.right),
		middle:     float64(out.middle),
		fullscreen: bool(out.fullscreen),
	}
}
