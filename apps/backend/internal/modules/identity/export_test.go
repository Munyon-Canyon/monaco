package identity

func (m *Module) Holdings() (stakes, balances any) { return m.stakes, m.balances }

func (m *Module) EnsuredPrivy() any {
	m.ensurePrivy()
	return m.privy
}

func (m *Module) Follows() any { return m.follows }

func (m *Module) Cabals() any { return m.cabals }

func (m *Module) Returns() any { return m.returns }

func (m *Module) Standings() any { return m.standings }

func (m *Module) DeleteChecks() (arriving, leaders any) { return m.arriving, m.leaders }
