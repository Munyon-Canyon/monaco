public enum AssetChartRange: String, Codable, CaseIterable, Sendable {
    case oneDay = "1D"
    case oneWeek = "1W"
    case oneMonth = "1M"
    case threeMonths = "3M"
    case oneYear = "1Y"
    case all = "ALL"

    public var label: String { rawValue }

    public var showsPreviousCloseBaseline: Bool { self == .oneDay }
}
