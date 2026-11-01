import Foundation

public struct HomeCabalRow: Equatable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let pictureURL: String?
    public let memberCount: Int
    public let ownershipText: String
    public let valueMicros: Int64
    public let returnBps: Int64?
    public let isFunded: Bool

    init(_ row: PortfolioSummary.Row) {
        id = row.id
        name = CabalHeaderCopy.shortName(row.name)
        pictureURL = row.pictureURL
        memberCount = row.memberCount
        ownershipText = PercentFormatter.format(basisPoints: row.ownershipBps, signed: false, fractionDigits: 0)
        valueMicros = max(0, row.valueMicros + row.arrivingMicros)
        isFunded = valueMicros > 0
        returnBps = isFunded ? row.returnBps : nil
    }

    public var membersText: String { memberCount == 1 ? "1 member" : "\(memberCount) members" }

    public func subtitle(votesText: String?) -> String {
        "\(votesText ?? membersText) · \(ownershipText) yours"
    }

    public func spoken(votesText: String?) -> String {
        subtitle(votesText: votesText).replacingOccurrences(of: " · ", with: ", ")
    }
}

public struct HomeCabalsContent: Equatable, Sendable {
    public static let shownCabals = 3

    public let cabals: [HomeCabalRow]
    public let invites: [HomeInviteRow]

    public var shown: [HomeCabalRow] { Array(cabals.prefix(Self.shownCabals)) }

    public var countText: String {
        invites.isEmpty ? "\(cabals.count)" : "\(cabals.count) · \(invites.count) invited"
    }
}

public enum HomeCabalsState: Equatable, Sendable {
    case loading
    case empty
    case cabals(HomeCabalsContent)
}

public enum HomeCabalList {
    public static func state(portfolio: PortfolioSummary, invites: [HomeInviteRow]?) -> HomeCabalsState {
        let content = HomeCabalsContent(cabals: portfolio.rows.map(HomeCabalRow.init), invites: invites ?? [])
        if !content.cabals.isEmpty || !content.invites.isEmpty { return .cabals(content) }
        return invites == nil ? .loading : .empty
    }
}
