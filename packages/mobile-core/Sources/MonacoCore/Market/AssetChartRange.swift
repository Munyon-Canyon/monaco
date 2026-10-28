public enum AssetChartRange: String, Codable, CaseIterable, Sendable {
    case oneHour = "1H"
    case oneDay = "1D"
    case oneWeek = "1W"
    case oneMonth = "1M"
    case threeMonths = "3M"
    case sixMonths = "6M"
    case oneYear = "1Y"
    case all = "ALL"

    public static let stockPage: [AssetChartRange] = [
        .oneHour, .oneDay, .oneWeek, .oneMonth, .threeMonths, .sixMonths, .all,
    ]

    public var label: String { self == .all ? "All" : rawValue }

    public var showsPreviousCloseBaseline: Bool { self == .oneDay }
}
