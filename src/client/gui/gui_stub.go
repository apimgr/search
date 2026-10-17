//go:build !gui || freebsd || netbsd || openbsd

package gui

import (
	"errors"

	"github.com/apimgr/search/src/client/api"
)

var errUnavailable = errors.New("native GUI support is not included in this build")

// Available reports whether this build includes native GUI support.
func Available() bool { return false }

// Launch reports that this build has no native GUI implementation.
func Launch(*api.Client) error { return errUnavailable }

// Configure reports that this build has no native GUI setup wizard.
func Configure() error { return errUnavailable }
