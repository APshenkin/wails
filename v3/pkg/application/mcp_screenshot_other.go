//go:build mcp && !ios && !android && (!darwin || server || purego)

package application

import (
	"errors"
	"time"
)

// mcpCaptureWindow is implemented for the macOS cgo backend only.
func mcpCaptureWindow(_ Window, _ mcpRect, _ float64, _, _ int, _ time.Duration) ([]byte, error) {
	return nil, errors.New("screenshot is not implemented on this platform yet; use screenshot_dom")
}
