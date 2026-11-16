//go:build mcp && darwin && !ios && !server && !purego

package application

/*
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#include "webview_window_darwin.h"

extern void mcpScreenshotDone(unsigned int id, void* data, int length, char* error);

static void mcpTakeScreenshot(void* nsWindow, unsigned int id, double scale, double maxSize,
		double maxPixels, double x, double y, double width, double height) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow<WailsWebviewWindow>* window = (NSWindow<WailsWebviewWindow>*)nsWindow;
		WKWebView* webView = window.webView;
		if (webView == nil) {
			mcpScreenshotDone(id, NULL, 0, "window has no web view");
			return;
		}

		CGRect rect = webView.bounds;
		if (width > 0 && height > 0) {
			CGFloat zoom = 1;
			if (@available(macOS 11.0, *)) {
				zoom = webView.pageZoom;
			}
			rect = CGRectIntersection(CGRectMake(x * zoom, y * zoom, width * zoom, height * zoom), webView.bounds);
			if (CGRectIsEmpty(rect)) {
				mcpScreenshotDone(id, NULL, 0, "the element lies outside the web view");
				return;
			}
		}

		CGFloat outputScale = MIN(scale > 0 ? scale : window.backingScaleFactor,
			MIN(maxSize / MAX(rect.size.width, rect.size.height), sqrt(maxPixels / (rect.size.width * rect.size.height))));

		WKSnapshotConfiguration* config = [[[WKSnapshotConfiguration alloc] init] autorelease];
		config.rect = rect;
		config.snapshotWidth = @(rect.size.width * outputScale / window.backingScaleFactor);

		[webView takeSnapshotWithConfiguration:config completionHandler:^(NSImage* image, NSError* error) {
			if (image == nil) {
				mcpScreenshotDone(id, NULL, 0, (char*)[[error localizedDescription] UTF8String]);
				return;
			}

			CGImageRef cgImage = [image CGImageForProposedRect:NULL context:nil hints:nil];
			NSBitmapImageRep* rep = [[[NSBitmapImageRep alloc] initWithCGImage:cgImage] autorelease];
			NSData* png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
			mcpScreenshotDone(id, (void*)[png bytes], (int)[png length], NULL);
		}];
	});
}
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"
)

type mcpScreenshotResult struct {
	png []byte
	err error
}

var (
	mcpScreenshotMu      sync.Mutex
	mcpScreenshotNextID  uint
	mcpScreenshotPending = map[uint]chan mcpScreenshotResult{}
)

func mcpCaptureWindow(window Window, clip mcpRect, scale float64, maxSize, maxPixels int, timeout time.Duration) ([]byte, error) {
	native := window.NativeWindow()
	if native == nil {
		return nil, errors.New("window has no native handle")
	}

	done := make(chan mcpScreenshotResult, 1)
	mcpScreenshotMu.Lock()
	mcpScreenshotNextID++
	id := mcpScreenshotNextID
	mcpScreenshotPending[id] = done
	mcpScreenshotMu.Unlock()

	defer func() {
		mcpScreenshotMu.Lock()
		delete(mcpScreenshotPending, id)
		mcpScreenshotMu.Unlock()
	}()

	C.mcpTakeScreenshot(native, C.uint(id), C.double(scale), C.double(maxSize), C.double(maxPixels),
		C.double(clip.X), C.double(clip.Y), C.double(clip.Width), C.double(clip.Height))

	select {
	case result := <-done:
		return result.png, result.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timed out after %s waiting for the web view snapshot", timeout)
	}
}

//export mcpScreenshotDone
func mcpScreenshotDone(id C.uint, data unsafe.Pointer, length C.int, errorMessage *C.char) {
	result := mcpScreenshotResult{png: C.GoBytes(data, length)}
	if errorMessage != nil {
		result.err = errors.New(C.GoString(errorMessage))
	} else if length == 0 {
		result.err = errors.New("the web view returned an empty snapshot")
	}

	mcpScreenshotMu.Lock()
	done := mcpScreenshotPending[uint(id)]
	mcpScreenshotMu.Unlock()

	if done != nil {
		done <- result
	}
}
