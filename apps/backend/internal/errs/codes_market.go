package errs

const (
	CodeAssetNotFound   Code = "asset_not_found"
	CodeAssetUntradable Code = "asset_untradable"
	CodeCalendarExpired Code = "calendar_expired"
)

func marketRows() map[Code]Row {
	return map[Code]Row{
		CodeAssetNotFound: {
			Name: "AssetNotFound", Kind: KindNotFound, Message: "That asset is not in the catalog.",
		},
		CodeAssetUntradable: {
			Name: "AssetUntradable", Kind: KindBlocked,
			Message: "This asset can't be traded right now",
		},
		CodeCalendarExpired: {
			Name: "CalendarExpired", Kind: KindInternal, Alert: true, Message: "Something went wrong.",
		},
	}
}
