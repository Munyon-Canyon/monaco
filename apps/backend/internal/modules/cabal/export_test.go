package cabal

import "github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"

func HTTPOf(m *Module) adapters.HTTP { return m.http() }
