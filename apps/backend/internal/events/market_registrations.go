package events

func marketRegistrations() []Registration {
	return []Registration{
		RegisterCore[PriceTick](TypePriceTick, 1),
		Register[AssetPriceMoved](TypeAssetPriceMoved, 1),
	}
}
