package social

import "github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"

func HTTPOf(m *Module) adapters.HTTP { return m.http() }
