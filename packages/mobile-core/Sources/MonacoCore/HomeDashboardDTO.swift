import Foundation

public struct HomeDashboardDTO: Codable, Equatable, Sendable {
    public let leaderboard: HomeLeaderboardSectionDTO

    public init(
        leaderboard: HomeLeaderboardSectionDTO
    ) {
        self.leaderboard = leaderboard
    }
}

public struct HomeLeaderboardSectionDTO: Codable, Equatable, Sendable {
    public let range: String
    public let people: [HomePeopleBoardRowDTO]

    public init(range: String, people: [HomePeopleBoardRowDTO]) {
        self.range = range
        self.people = people
    }
}

public enum HomeLeaderboardRange: String, CaseIterable, Sendable {
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
}

func monacoISO8601JSONDecoder() -> JSONDecoder {
    let decoder = JSONDecoder()
    decoder.dateDecodingStrategy = .custom { decoder in
        let container = try decoder.singleValueContainer()
        let raw = try container.decode(String.self)
        if let date = SharedFormatters.iso8601Date(from: raw) {
            return date
        }
        throw DecodingError.dataCorruptedError(
            in: container,
            debugDescription: "Expected ISO8601 date, got \(raw)"
        )
    }
    return decoder
}
