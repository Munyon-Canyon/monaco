public enum HomeVariant: Equatable, Sendable {
    case empty
    case cashOnly
    case invested

    public init(summary: PortfolioSummary, cashMicros: Int64) {
        if summary.isEmpty && cashMicros <= 0 {
            self = .empty
        } else if summary.totalMicros <= 0 {
            self = .cashOnly
        } else {
            self = .invested
        }
    }

    public var showsMoneyStrip: Bool { self != .empty }
    public var showsProposals: Bool { self == .invested }
}
