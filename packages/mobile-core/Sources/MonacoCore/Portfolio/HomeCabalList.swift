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
    public let note: StakeValueNote?

    init(_ row: PortfolioSummary.Row) {
        id = row.id
        name = CabalHeaderCopy.shortName(row.name)
        pictureURL = row.pictureURL
        memberCount = row.memberCount
        ownershipText = PercentFormatter.format(basisPoints: row.ownershipBps, signed: false, fractionDigits: 0)
        valueMicros = max(0, row.valueMicros + row.arrivingMicros)
        isFunded = valueMicros > 0
        note = StakeValueNote.make(
            valueMicros: row.valueMicros, arrivingMicros: row.arrivingMicros, isStale: row.isStale,
            valuedAt: row.valuedAt)
        returnBps = isFunded && note != .settling ? row.returnBps : nil
    }

    public var returnText: String? { returnBps.map { PortfolioSummary.percent($0) } }

    public var membersText: String { memberCount == 1 ? "1 member" : "\(memberCount) members" }

    public func subtitle(votesText: String?) -> String {
        "\(votesText ?? membersText) · \(ownershipText) yours"
    }

    public func spoken(votesText: String?) -> String {
        subtitle(votesText: votesText).replacingOccurrences(of: " · ", with: ", ")
    }

    public func noteText(now: Date, calendar: Calendar = .current) -> String? {
        note?.text(now: now, calendar: calendar)
    }
}

public struct HomeCabalsContent: Equatable, Sendable {
    public static let shownCabals = 3

    public let cabals: [HomeCabalRow]
    public let invites: [HomeInviteRow]
    public let totalMicros: Int64

    public var shown: [HomeCabalRow] { Array(cabals.prefix(Self.shownCabals)) }

    public var countText: String {
        invites.isEmpty ? "\(cabals.count)" : "\(cabals.count) · \(invites.count) invited"
    }

    public var totalText: String? {
        UsdAmountFormatter.flooredToCents(totalMicros) > 0 ? HomeFigureCopy.partFigure(micros: totalMicros) : nil
    }

    public var headerText: String { totalText ?? countText }

    public var headerSpoken: String {
        guard let totalText else { return countText }
        let parts: [Substring] = totalText.dropFirst().split(separator: ".", omittingEmptySubsequences: false)
        let whole = String(parts[0])
        let cents = parts.count > 1 ? Int(parts[1]) ?? 0 : 0
        var spoken = "\(whole) \(whole == "1" ? "dollar" : "dollars")"
        if cents > 0 { spoken += " \(cents) \(cents == 1 ? "cent" : "cents")" }
        return spoken
    }
}

public enum HomeCabalsState: Equatable, Sendable {
    case loading
    case empty
    case cabals(HomeCabalsContent)
}

public enum HomeCabalList {
    public static func state(portfolio: PortfolioSummary, invites: [HomeInviteRow]?) -> HomeCabalsState {
        let content = HomeCabalsContent(
            cabals: portfolio.rows.map(HomeCabalRow.init), invites: invites ?? [], totalMicros: portfolio.totalMicros)
        if !content.cabals.isEmpty || !content.invites.isEmpty { return .cabals(content) }
        return invites == nil ? .loading : .empty
    }
}
