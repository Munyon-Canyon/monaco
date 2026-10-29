import Foundation
import MonacoAPI

public struct AssetHeldRowFacts: Equatable, Sendable, Identifiable {
    public let cabalID: String
    public let cabalName: String
    public let pictureURL: String?
    public let title: String
    public let subtitle: String
    public let valueMicros: Int64
    public let returnText: String?

    public var id: String { cabalID }

    public init(_ row: Components.Schemas.HeldRow) {
        let holding = row.holding
        cabalID = row.cabal.id
        cabalName = row.cabal.name
        pictureURL = row.cabal.pictureUrl
        title = row.cabal.name
        var shares = Substring(holding.units)
        if shares.contains(".") {
            while shares.last == "0" { shares = shares.dropLast() }
            if shares.last == "." { shares = shares.dropLast() }
        }
        subtitle = "\(shares) sh · \(max(Int(holding.weightBps), 0) / 100)% of treasury"
        valueMicros = holding.valueMicros
        let (scaled, overflow) = holding.pnlMicros.multipliedReportingOverflow(by: 10_000)
        returnText =
            holding.costBasisMicros > 0 && !overflow
            ? PercentFormatter.format(
                basisPoints: scaled / holding.costBasisMicros, signed: true, fractionDigits: 1)
            : nil
    }

    public var valueText: String { UsdAmountFormatter.format(micros: valueMicros) }
}

public struct AssetOpenProposalRowFacts: Equatable, Sendable, Identifiable {
    public let proposalID: String
    public let cabalID: String
    public let cabalName: String
    public let pictureURL: String?
    public let title: String
    public let subtitle: String
    public let trailing: String

    public var id: String { proposalID }

    public init?(_ item: Components.Schemas.FeedItem, now: Date) {
        guard let proposalID = item.proposalId, let cabalID = item.cabalId, let cabalName = item.cabalName,
            let tally = item.tally, let expiresAt = item.expiresAt
        else { return nil }
        self.proposalID = proposalID
        self.cabalID = cabalID
        self.cabalName = cabalName
        pictureURL = item.cabalPictureUrl
        let action = item.action == "sell" ? "Sell" : "Buy"
        title = "\(cabalName) · \(action)" + (item.usdcMicros.map { " \(Self.dollars($0))" } ?? "")
        subtitle = ProposalVoteCopy.statusLine(
            tally: ProposalTally(yes: tally.yes, no: tally.no, voters: tally.voters, needed: tally.needed),
            replies: Int(item.commentCount), expiresAt: expiresAt, now: now)
        trailing = "✓ \(tally.yes) · ✕ \(tally.no)"
    }

    private static func dollars(_ micros: Int64) -> String {
        let text = UsdAmountFormatter.format(micros: micros)
        return text.hasSuffix(".00") ? String(text.dropLast(3)) : text
    }
}
