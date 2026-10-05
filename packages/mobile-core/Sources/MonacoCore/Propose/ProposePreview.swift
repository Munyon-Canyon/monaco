import MonacoAPI

public struct ProposePreview: Equatable, Sendable {
    public let quoteOutAmount: Int64?
    public let advisoryCode: String?
    public let advisoryMessage: String?
    public let potValueMicros: Int64

    public init(_ preview: Components.Schemas.TradePreview) {
        quoteOutAmount = preview.quoteOutAmount
        advisoryCode = preview.advisoryCode
        advisoryMessage = preview.advisoryMessage
        potValueMicros = preview.potValueMicros
    }

    public var maxMicros: Int64 { potValueMicros }
    public var potHelperText: String { "The pot has \(UsdAmountFormatter.format(micros: potValueMicros))" }

    public func message(assetName: String, isSell: Bool = false) -> String? {
        switch advisoryCode {
        case "pot_exceeded": "More than the pot has"
        case "insufficient_funds" where isSell: "More than the cabal holds"
        case "no_route", "asset_untradable":
            isSell
                ? "Can't sell \(assetName) right now. Try a smaller amount."
                : "Can't buy \(assetName) right now. Try a smaller amount or another stock."
        default: advisoryMessage
        }
    }

    public func reviewEnabled(amountMicros: Int64, isLoading: Bool, assetName: String, isSell: Bool = false) -> Bool {
        amountMicros > 0 && !isLoading && message(assetName: assetName, isSell: isSell) == nil
    }
}
