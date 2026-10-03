package treasury

type CatalogNames = catalogNames

func (m *Module) Reads() (members, users any) { return m.members, m.users }
