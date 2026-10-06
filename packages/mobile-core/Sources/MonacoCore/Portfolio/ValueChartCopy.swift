extension LeaderboardRange {
    public var shortHistoryLine: String {
        switch self {
        case .oneHour: "Not enough history for the last hour yet"
        case .oneDay: "Not enough history for the last day yet"
        case .oneWeek: "Not enough history for the last week yet"
        case .oneMonth: "Not enough history for the last month yet"
        case .all: "Not enough history yet"
        }
    }
}
