package market

import "github.com/monaco/monaco/apps/backend/internal/modules/market/app"

func HotMints(m *Module) []app.HotMints { return m.hot }

func WithHolders(m *Module, h app.HolderCounts) { m.holders = h }

func Holders(m *Module) app.HolderCounts { return m.holders }
