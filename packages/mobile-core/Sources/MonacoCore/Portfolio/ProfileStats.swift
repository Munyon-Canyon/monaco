import MonacoAPI

public struct ProfileStats: Equatable, Sendable {
    public let inCabals: String
    public let returnDollars: String
    public let returnPercent: String?
    public let cabals: String
    public let cabalsLabel: String

    public init(_ portfolio: Components.Schemas.MyPortfolio) {
        inCabals = UsdAmountFormatter.format(micros: portfolio.totalValueMicros)
        returnDollars = UsdAmountFormatter.format(signedMicros: portfolio.pnlMicros)
        returnPercent = portfolio.returnBps.map {
            PercentFormatter.format(basisPoints: $0, signed: true, fractionDigits: 1)
        }
        let count = portfolio.cabals.count
        cabals = "\(count)"
        cabalsLabel = count == 1 ? "Cabal" : "Cabals"
    }
}
