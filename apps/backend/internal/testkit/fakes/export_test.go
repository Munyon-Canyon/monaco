package fakes

import (
	"io/fs"
	"log/slog"
)

func NewFrom(fsys fs.FS, root string) *Server { return newFrom(fsys, root) }

func (s *Server) SetLog(l *slog.Logger) { s.log = l }
