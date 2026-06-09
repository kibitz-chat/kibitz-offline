//go:build tools

// This file exists only to keep golang.org/x/mobile in the module dependency
// graph — `gomobile bind` requires it present even though no runtime code
// imports it. The `tools` build tag excludes it from normal builds.
package main

import (
	_ "golang.org/x/mobile/bind"
)
