// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package web

import (
	"embed"
	"io/fs"
)

// StaticFiles is the website bundled into the binary.
//
//go:embed all:static
var staticFiles embed.FS

// FS returns the static site rooted at / (index.html at the top).
func FS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
