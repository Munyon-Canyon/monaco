import Foundation

public struct HomeDashboardDTO: Codable, Equatable, Sendable {
    public let netWorthUsd: String
    public let netWorthDollarPnl: String
    public let netWorthPercentReturn: String?
    public let myGroups: [HomeMyGroupRowDTO]
    public let pnlSeries1H: [HomePnLSeriesPointDTO]
    public let leaderboard: HomeLeaderboardSectionDTO

    public init(
        netWorthUsd: String,
        netWorthDollarPnl: String,
        netWorthPercentReturn: String?,
        myGroups: [HomeMyGroupRowDTO],
        pnlSeries1H: [HomePnLSeriesPointDTO],
        leaderboard: HomeLeaderboardSectionDTO
    ) {
        self.netWorthUsd = netWorthUsd
        self.netWorthDollarPnl = netWorthDollarPnl
        self.netWorthPercentReturn = netWorthPercentReturn
        self.myGroups = myGroups
        self.pnlSeries1H = pnlSeries1H
        self.leaderboard = leaderboard
    }
}

public struct HomeMyGroupRowDTO: Codable, Equatable, Sendable, Identifiable {
    public let groupID: String
    public let name: String
    public let equityUsd: String
    public let slicePercent: String
    public let dollarPnl: String
    public let percentReturn: String?
    /// The cabal's picture. Nil when it has none, and the mark falls back
    /// to its tinted initials.
    public let pictureUrl: String?

    public var id: String { groupID }

    public init(
        groupID: String,
        name: String,
        equityUsd: String,
        slicePercent: String,
        dollarPnl: String,
        percentReturn: String?,
        pictureUrl: String? = nil
    ) {
        self.groupID = groupID
        self.name = name
        self.equityUsd = equityUsd
        self.slicePercent = slicePercent
        self.dollarPnl = dollarPnl
        self.percentReturn = percentReturn
        self.pictureUrl = pictureUrl
    }

    enum CodingKeys: String, CodingKey {
        case groupID = "groupId"
        case name
        case equityUsd
        case slicePercent
        case dollarPnl
        case percentReturn
        case pictureUrl
    }
}

public struct HomePnLSeriesPointDTO: Codable, Equatable, Sendable, Identifiable {
    public let ts: Date
    public let equityUsd: String
    public let dollarPnl: String

    public var id: TimeInterval { ts.timeIntervalSince1970 }

    public var chartValue: Double {
        let cleaned = dollarPnl.replacingOccurrences(of: "+", with: "")
        return Double(cleaned) ?? 0
    }

    public init(ts: Date, equityUsd: String, dollarPnl: String) {
        self.ts = ts
        self.equityUsd = equityUsd
        self.dollarPnl = dollarPnl
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

public struct HomePnLSeriesDTO: Codable, Equatable, Sendable {
    public let points: [HomePnLSeriesPointDTO]

    public init(points: [HomePnLSeriesPointDTO]) {
        self.points = points
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
