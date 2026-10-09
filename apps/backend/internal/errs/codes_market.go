package errs

const (
	CodeAssetNotFound   Code = "asset_not_found"
	CodeAssetUntradable Code = "asset_untradable"
	CodeAssetPaused     Code = "asset_paused"
	CodeCalendarExpired Code = "calendar_expired"
)

func (codeFiles) Market() map[Code]Row {
	return map[Code]Row{
		CodeAssetNotFound: {
			Name: "AssetNotFound", Kind: KindNotFound, Message: "That asset is not in the catalog.",
		},
		CodeAssetUntradable: {
			Name: "AssetUntradable", Kind: KindBlocked,
			Message: "This asset can't be traded right now.",
		},
		CodeAssetPaused: {
			Name: "AssetPaused", Kind: KindBlocked,
			Message: "This stock can't be traded right now.",
		},
		CodeCalendarExpired: {
			Name: "CalendarExpired", Kind: KindInternal, Alert: true, Message: "Something went wrong.",
		},
	}
}
