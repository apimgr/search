package closeutil

import (
	"io"
	"log/slog"
)

// Close closes a resource and reports failures from deferred cleanup.
func Close(resource io.Closer) {
	if err := resource.Close(); err != nil {
		slog.Warn("resource close failed", "error", err)
	}
}
