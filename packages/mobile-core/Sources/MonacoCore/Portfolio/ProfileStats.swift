import MonacoAPI

public struct ProfileStats: Equatable, Sendable {
    public let inCabals: String
    public let allTime: String
    public let cabals: String
    public let allTimePnl: String

    public init(_ portfolio: Components.Schemas.MyPortfolio) {
        inCabals = UsdAmountFormatter.format(micros: portfolio.totalValueMicros)
        allTimePnl = UsdAmountFormatter.format(signedMicros: portfolio.pnlMicros)
        if let returnBps = portfolio.returnBps {
            let percent = PercentFormatter.format(basisPoints: returnBps, signed: true, fractionDigits: 1)
            allTime = "\(allTimePnl) · \(percent)"
        } else {
            allTime = allTimePnl
        }
        cabals = "\(portfolio.cabals.count)"
    }
}
