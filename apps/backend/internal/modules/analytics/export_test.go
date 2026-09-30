package analytics

import "github.com/monaco/monaco/apps/backend/internal/platform/module"

func NewWithExports(d module.Deps, r *Registry) *Module { return newModule(d, r) }
