import Foundation

extension LeaderboardRange {
    public var shortHistoryLine: String {
        switch self {
        case .oneHour: "Not enough history for the last hour yet"
        case .oneDay: "Not enough history for the last day yet"
        case .oneWeek: "Not enough history for the last week yet"
        case .oneMonth: "Not enough history for the last month yet"
        case .threeMonths: "Not enough history for the last 3 months yet"
        case .sixMonths: "Not enough history for the last 6 months yet"
        case .all: "Not enough history yet"
        }
    }
}

extension LeaderboardRange {
    public static let homePeriods: [LeaderboardRange] = [.oneWeek, .oneMonth, .all]
    public static let figurePeriods: [LeaderboardRange] = [
        .oneHour, .oneDay, .oneWeek, .oneMonth, .threeMonths, .sixMonths, .all,
    ]

    public var returnPeriodPhrase: String {
        switch self {
        case .oneHour: "past hour"
        case .oneDay: "past day"
        case .oneWeek: "past week"
        case .oneMonth: "past month"
        case .threeMonths: "past 3 months"
        case .sixMonths: "past 6 months"
        case .all: "all time"
        }
    }
}

public enum PortfolioStaleCopy {
    public static let title = "Prices delayed"

    public static func line(lastUpdated: Date?, now: Date) -> String {
        guard let lastUpdated else { return "Last updated a while ago." }
        let minutes = max(Int(now.timeIntervalSince(lastUpdated) / 60), 1)
        return "Last updated \(minutes) minute\(minutes == 1 ? "" : "s") ago."
    }
}
