import Foundation
import MonacoAPI

public enum FeedVoteCard {
    public static func row(for item: Components.Schemas.FeedItem) -> HomePendingVote? {
        guard item.kind == "proposal", let proposalID = item.proposalId, let cabalID = item.cabalId,
            let cabalName = item.cabalName, let action = item.action, ["buy", "sell"].contains(action),
            let tally = item.tally, let expiresAt = item.expiresAt
        else { return nil }
        return HomePendingVote(
            id: proposalID, cabalID: cabalID, cabalName: cabalName, kind: action, symbol: item.symbol,
            assetName: item.assetName, logoURL: item.assetLogoUrl.flatMap(URL.init(string:)),
            usdcMicros: item.usdcMicros, tokenAmount: item.tokenAmountValue, tokenDecimals: item.tokenDecimals,
            tally: ProposalTally(yes: tally.yes, no: tally.no, voters: tally.voters, needed: tally.needed),
            replyCount: 0, expiresAt: expiresAt)
    }

    public static func ballot(of item: Components.Schemas.FeedItem) -> ProposalBallot? {
        item.myBallot.flatMap { ProposalBallot(rawValue: $0.rawValue) }
    }

    public static func isVotable(_ item: Components.Schemas.FeedItem) -> Bool {
        row(for: item) != nil && (item.canVote || ballot(of: item) != nil)
    }

    public static func readOnlyLine(for item: Components.Schemas.FeedItem, now: Date) -> String? {
        guard item.status == nil || item.status == "open", let row = row(for: item) else { return nil }
        return ProposalVoteCopy.cardLine(
            cabalName: row.cabalName, tally: row.tally, replies: nil, expiresAt: row.expiresAt, now: now
        ).short
    }
}
