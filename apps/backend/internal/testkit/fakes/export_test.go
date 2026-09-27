package fakes

import "io/fs"

func NewFrom(fsys fs.FS, root string) *Server { return newFrom(fsys, root) }
