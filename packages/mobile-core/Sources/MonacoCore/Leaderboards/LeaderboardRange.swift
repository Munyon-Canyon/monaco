import MonacoAPI

public enum LeaderboardRange: String, CaseIterable, Sendable {
    case oneHour = "1H"
    case oneDay = "1D"
    case oneWeek = "1W"
    case oneMonth = "1M"
    case threeMonths = "3M"
    case sixMonths = "6M"
    case all = "ALL"

    public static let allCases: [LeaderboardRange] = [.oneHour, .oneDay, .oneWeek, .oneMonth, .all]

    public var label: String {
        switch self {
        case .oneHour: "1H"
        case .oneDay: "1D"
        case .oneWeek: "1W"
        case .oneMonth: "1M"
        case .threeMonths: "3M"
        case .sixMonths: "6M"
        case .all: "All"
        }
    }

    public var windowPhrase: String {
        switch self {
        case .oneHour: "the past hour"
        case .oneDay: "the past day"
        case .oneWeek: "the past week"
        case .oneMonth: "the past month"
        case .threeMonths: "the past 3 months"
        case .sixMonths: "the past 6 months"
        case .all: "all time"
        }
    }

    public var scrubLabelRange: AssetChartRange {
        switch self {
        case .oneHour, .oneDay: .oneDay
        case .oneWeek: .oneWeek
        case .oneMonth: .oneMonth
        case .threeMonths: .threeMonths
        case .sixMonths: .sixMonths
        case .all: .all
        }
    }

    public init(_ generated: Components.Parameters.LeaderboardRange) {
        switch generated {
        case ._1h: self = .oneHour
        case ._1d: self = .oneDay
        case ._1w: self = .oneWeek
        case ._1m: self = .oneMonth
        case .all: self = .all
        }
    }

    public var generated: Components.Parameters.LeaderboardRange {
        switch self {
        case .oneHour: ._1h
        case .oneDay: ._1d
        case .oneWeek: ._1w
        case .oneMonth: ._1m
        case .all, .threeMonths, .sixMonths: .all
        }
    }

    public var valueHistory: Components.Parameters.ValueHistoryRange {
        switch self {
        case .oneHour: ._1h
        case .oneDay: ._1d
        case .oneWeek: ._1w
        case .oneMonth: ._1m
        case .threeMonths: ._3m
        case .sixMonths: ._6m
        case .all: .all
        }
    }
}
