import Foundation

public enum CabalHoldingCopy {
    public static let fallbackTitle = "Cabal asset holding"
    public static let positionValue = "Position value"
    public static let allTime = "All time"
    public static let noSales = "No sales yet"
    public static let emptyTitle = "No position yet"
    public static let emptyBody = "Completed trades in this cabal appear here."
    public static let tradeHistory = "Trade history"
    public static let noTrades = "No completed trades"
    public static let olderTrades = "Show older trades"
    public static let tradeFailed = "Couldn't load trade history."
    public static let related = "Related proposals"
    public static let details = "Position details"
    public static let viewMarket = "View token market"
    public static let errorTitle = "Couldn't load this view"
    public static let errorBody =
        "Your existing information hasn't been changed. Try again when your connection is available."
    public static let tryAgain = "Try again"
    public static let refreshFailed = "Couldn't refresh this holding."
    public static let privateTitle = "This holding is private"
    public static let missing = "—"

    public enum Stat {
        public static let units = "Units"
        public static let averageBuy = "Average buy"
        public static let averageSell = "Average sell"
        public static let costBasis = "Cost basis"
        public static let realized = "Realized P&L · total"
        public static let unrealized = "Unrealized P&L"
    }

    public enum Detail {
        public static let asset = "Asset"
        public static let scope = "Scope"
        public static let averageBuy = "Average buy"
        public static let averageBuyValue = "Quantity-weighted acquisition cost"
        public static let pnlScope = "P&L scope"
        public static let pnlScopeValue = "Realized + unrealized"
    }

    public static func title(_ name: String) -> String {
        "\(name) holding"
    }

    public static func averageBuyDetail(_ price: String) -> String {
        "\(price) · \(Detail.averageBuyValue)"
    }

    public static func pnlLabel(_ period: String) -> String {
        "Position P&L · \(period)"
    }

    public static func stale(_ minutes: Int) -> String {
        "Holding value last updated \(minutes) \(minutes == 1 ? "minute" : "minutes") ago."
    }

    public static func utc(_ cabal: String) -> String {
        "\(cabal) · Times shown in UTC"
    }

    public static func noProposals(_ ticker: String) -> String {
        "No open proposals for \(ticker)"
    }

    public static func privateBody(_ cabal: String) -> String {
        "Only members of \(cabal) can see its positions."
    }

    public static func header(ticker: String, cabal: String) -> String {
        "\(ticker) · \(cabal)"
    }

    public static func headerSpoken(name: String, ticker: String, cabal: String) -> String {
        "\(name), \(ticker), held by \(cabal)"
    }

    public static func tradeTitle(isBuy: Bool, units: String) -> String {
        "\(isBuy ? "Buy" : "Sell") · \(units) units"
    }

    public static func perUnit(_ price: String) -> String {
        "\(price) per unit"
    }
}
