package identity

func (m *Module) Holdings() (stakes, balances any) { return m.stakes, m.balances }

func (m *Module) EnsuredPrivy() any {
	m.ensurePrivy()
	return m.privy
}

func (m *Module) Follows() any { return m.follows }
