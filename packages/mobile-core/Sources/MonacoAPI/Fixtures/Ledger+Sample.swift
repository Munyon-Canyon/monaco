#if DEBUG
import Foundation

extension Components.Schemas.Ledger {
    private typealias User = Components.Schemas.LedgerUser
    private typealias Cabal = Components.Schemas.LedgerCabal
    private typealias Call = Components.Schemas.LedgerCall

    private static let start = Date(timeIntervalSince1970: 1_791_766_800)
    private static let degens = Cabal(id: "00000000-0000-7000-8000-00000000c003", name: "Degens", pictureUrl: nil)
    private static let acCapital = Cabal(
        id: "00000000-0000-7000-8000-00000000c001", name: "AC Capital", pictureUrl: nil)
    private static let techTitans = Cabal(
        id: "00000000-0000-7000-8000-00000000c002", name: "Tech Titans", pictureUrl: nil)
    private static let christelle = User(
        id: "00000000-0000-7000-8000-00000000a001", name: "Christelle Dupont", pictureUrl: nil)

    private static func call(
        _ n: Int, _ cabal: Cabal, _ kind: Call.KindPayload, _ symbol: String, yes: Bool, move: Int32, right: Bool
    ) -> Call {
        Call(
            proposalId: "00000000-0000-7000-8000-00000000b00\(n)", cabal: .init(id: cabal.id, name: cabal.name),
            kind: kind, symbol: symbol, yourChoice: yes ? .yes : .no, moveBps: move, right: right,
            decidedAt: start.addingTimeInterval(TimeInterval(n) * 3_600))
    }

    private static let sellAmzn = call(1, degens, .sell, "AMZN", yes: true, move: 420, right: true)
    private static let buyNvda = call(2, acCapital, .buy, "NVDA", yes: true, move: 180, right: true)
    private static let buyTsla = call(3, techTitans, .buy, "TSLA", yes: false, move: -240, right: false)

    private static func make(
        returnBps: Int64?, calls: [Call], withBest: Bool, withMiss: Bool, withReply: Bool
    ) -> Self {
        Self(
            week: "2026-W42", labelStart: "2026-10-12", labelEnd: "2026-10-18", windowStart: start,
            windowEnd: start.addingTimeInterval(7 * 86_400), stakeReturnBps: returnBps,
            cabals: [degens, acCapital, techTitans], voted: Int32(calls.count), eligible: 3,
            right: Int32(calls.filter(\.right).count), resolved: Int32(calls.count),
            sharpest: .init(user: christelle, rateBps: 10_000, right: 2, resolved: 2),
            bestCall: withBest
                ? .init(
                    proposalId: sellAmzn.proposalId, cabal: .init(id: degens.id, name: degens.name), kind: .sell,
                    symbol: "AMZN", yourChoice: .yes, moveBps: 420, right: true, decidedAt: sellAmzn.decidedAt) : nil,
            miss: withMiss
                ? .init(
                    proposalId: buyTsla.proposalId, cabal: .init(id: techTitans.id, name: techTitans.name),
                    kind: .buy, symbol: "TSLA", yourChoice: .no, moveBps: -240, right: false,
                    decidedAt: buyTsla.decidedAt) : nil,
            calls: calls,
            topReply: withReply
                ? .init(
                    commentId: "00000000-0000-7000-8000-00000000d001", proposalId: buyNvda.proposalId,
                    proposalLabel: "Buy NVDA", author: christelle, excerpt: "Earnings are the catalyst.",
                    reactionCount: 5, topEmoji: "🔥", topEmojiCount: 3) : nil,
            series: [], markers: [])
    }

    public static let week42 = make(
        returnBps: 140, calls: [sellAmzn, buyNvda, buyTsla], withBest: true, withMiss: true, withReply: true)

    public static let noBestCall = make(
        returnBps: nil, calls: [buyTsla], withBest: false, withMiss: true, withReply: false)

    public static let loss = make(
        returnBps: -80, calls: [sellAmzn, buyNvda, buyTsla], withBest: true, withMiss: true, withReply: true)
}
#endif
