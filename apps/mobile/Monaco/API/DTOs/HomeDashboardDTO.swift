import Foundation
import MonacoCore

struct HomeDashboardDTO: Codable, Equatable {
    let leaderboard: HomeLeaderboardSectionDTO
}

struct HomeLeaderboardSectionDTO: Codable, Equatable {
    let range: String
    let people: [HomePeopleBoardRowDTO]
}

enum HomeLeaderboardRange: String, CaseIterable {
    case oneHour = "1H"
    case oneDay = "1D"
    case oneWeek = "1W"
    case oneMonth = "1M"
    case all = "ALL"

    var label: String {
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
