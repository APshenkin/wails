//go:build mcp && darwin && !ios && !server && !purego

package application

/*
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#include "webview_window_darwin.h"

extern void mcpScreenshotDone(unsigned int id, void* data, int length, char* error);

static void mcpTakeScreenshot(void* nsWindow, unsigned int id, double scale) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow<WailsWebviewWindow>* window = (NSWindow<WailsWebviewWindow>*)nsWindow;
		WKWebView* webView = window.webView;
		if (webView == nil) {
			mcpScreenshotDone(id, NULL, 0, "window has no web view");
			return;
		}

		WKSnapshotConfiguration* config = [[[WKSnapshotConfiguration alloc] init] autorelease];
		config.snapshotWidth = @(webView.bounds.size.width * scale / window.backingScaleFactor);

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

// mcpCaptureWindow renders the window's visible web content to PNG, at scale
// output pixels per point of the web view.
func mcpCaptureWindow(window Window, scale float64, timeout time.Duration) ([]byte, error) {
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

	C.mcpTakeScreenshot(native, C.uint(id), C.double(scale))

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
