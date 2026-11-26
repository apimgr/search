//go:build !gui || freebsd || netbsd || openbsd

package gui

import (
	"errors"

	"github.com/apimgr/search/src/client/api"
	"github.com/apimgr/search/src/client/surface"
)

var errUnavailable = errors.New(surface.Text("client.surface.gui_unavailable"))

// Available reports whether this build includes native GUI support.
func Available() bool { return false }

// Screens returns the registered command screens for mapping and help tools.
func Screens() []surface.Command { return surface.Commands() }

// Launch reports that this build has no native GUI implementation.
func Launch(*api.Client) error { return errUnavailable }

// Configure reports that this build has no native GUI setup wizard.
func Configure() error { return errUnavailable }
