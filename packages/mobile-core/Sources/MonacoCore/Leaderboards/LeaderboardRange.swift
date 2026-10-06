import MonacoAPI

public enum LeaderboardRange: String, CaseIterable, Sendable {
    case oneHour = "1H"
    case oneDay = "1D"
    case oneWeek = "1W"
    case oneMonth = "1M"
    case all = "ALL"

    public var label: String {
        switch self {
        case .oneHour: "1H"
        case .oneDay: "1D"
        case .oneWeek: "1W"
        case .oneMonth: "1M"
        case .all: "All"
        }
    }

    public var windowPhrase: String {
        switch self {
        case .oneHour: "the past hour"
        case .oneDay: "the past day"
        case .oneWeek: "the past week"
        case .oneMonth: "the past month"
        case .all: "all time"
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
        case .all: .all
        }
    }

    public var valueHistory: Components.Parameters.ValueHistoryRange {
        switch self {
        case .oneHour: ._1h
        case .oneDay: ._1d
        case .oneWeek: ._1w
        case .oneMonth: ._1m
        case .all: .all
        }
    }
}
