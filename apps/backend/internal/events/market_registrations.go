package events

func marketRegistrations() []Registration {
	return []Registration{
		RegisterCore[PriceTick](TypePriceTick, 1),
	}
}
