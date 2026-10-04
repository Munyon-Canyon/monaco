#if DEBUG
import Foundation

extension Components.Schemas.TradePreview {
    public static let proposalPreviewClean = Self(quoteOutAmount: 21_000_000, potValueMicros: 100_000_000)
    public static let proposalPreviewPotExceeded = Self(
        advisoryCode: "pot_exceeded", advisoryMessage: "More than the pot has", potValueMicros: 100_000_000
    )
    public static let proposalPreviewNoRoute = Self(
        advisoryCode: "no_route", advisoryMessage: "No route.", potValueMicros: 100_000_000
    )
    public static let proposalPreviewAssetUntradable = Self(
        advisoryCode: "asset_untradable", advisoryMessage: "Not tradable.", potValueMicros: 100_000_000
    )
}
#endif
