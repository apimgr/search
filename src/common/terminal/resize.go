package terminal

import (
	"sync"
)

// ResizeHandler manages terminal resize events
type ResizeHandler struct {
	mu        sync.RWMutex
	callbacks []func(Size)
	size      Size
}

// NewResizeHandler creates a new resize handler
func NewResizeHandler() *ResizeHandler {
	h := &ResizeHandler{
		size: GetSize(),
	}
	return h
}

// OnResize registers a callback for resize events
func (h *ResizeHandler) OnResize(callback func(Size)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbacks = append(h.callbacks, callback)
}

// CurrentSize returns the current terminal size
func (h *ResizeHandler) CurrentSize() Size {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.size
}

// Refresh manually refreshes the terminal size
func (h *ResizeHandler) Refresh() {
	newSize := GetSize()

	h.mu.Lock()
	oldSize := h.size
	h.size = newSize
	callbacks := make([]func(Size), len(h.callbacks))
	copy(callbacks, h.callbacks)
	h.mu.Unlock()

	// Only notify if size changed
	if oldSize.Cols != newSize.Cols || oldSize.Rows != newSize.Rows {
		for _, cb := range callbacks {
			cb(newSize)
		}
	}
}
