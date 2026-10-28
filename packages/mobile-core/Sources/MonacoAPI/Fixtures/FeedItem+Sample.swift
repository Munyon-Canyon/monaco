#if DEBUG
import Foundation

extension Components.Schemas.FeedItem {
    public static let sampleCabalID = "00000000-0000-7000-8000-0000000fc001"

    public static let samples: [Self] = [
        sample(
            1, kind: "proposal", refType: "proposals", cabalID: sampleCabalID, actor: 1, symbol: "AAPLx",
            title: "Maya proposed buying $500 of AAPLx", detail: "Weekend investors",
            body: "Earnings beat three quarters running and services keep growing. A small slice before the call.",
            status: "open", comments: 6,
            reactions: [.init(emoji: "💪", count: 3, mine: false), .init(emoji: "👀", count: 2, mine: false)],
            vote: Vote(yes: 4, no: 1, voters: 8, needed: 5, endsInHours: 4), minutesAgo: 4),
        sample(
            2, kind: "trade", refType: "swaps", cabalID: sampleCabalID, actor: 2, symbol: "NVDAx",
            title: "Weekend investors bought $250 of NVDAx", detail: "NVIDIA", tone: "positive", comments: 3,
            reactions: [.init(emoji: "💰", count: 4, mine: false), .init(emoji: "🙌", count: 2, mine: false)],
            minutesAgo: 42),
        sample(
            3, kind: "price_move", refType: "assets", cabalID: nil, actor: nil, symbol: "TSLAx",
            title: "TSLAx fell 6.2% today", detail: "Tesla", tone: "negative", minutesAgo: 95),
        sample(
            4, kind: "cabal_created", refType: "cabals", cabalID: sampleCabalID, actor: 3, symbol: nil,
            title: "Jordan started Weekend investors", detail: "Open to anyone",
            reactions: [.init(emoji: "🎉", count: 5, mine: true), .init(emoji: "👏", count: 2, mine: false)],
            minutesAgo: 60 * 5),
        sample(
            5, kind: "member_joined", refType: "cabal_members", cabalID: sampleCabalID, actor: 4, symbol: nil,
            title: "Sam joined Weekend investors", detail: nil, comments: 1, minutesAgo: 60 * 26),
    ]

    public static let publicProposals: [Self] = [
        sample(
            6, kind: "proposal", refType: "proposals", cabalID: publicCabalID, actor: 1, symbol: "AMZNx",
            title: "Maya proposed buying $250 of AMZNx", detail: "Pilates", status: "open", comments: 2,
            vote: Vote(yes: 3, no: 0, voters: 7, needed: 4, endsInHours: 23), minutesAgo: 30),
        sample(
            7, kind: "proposal", refType: "proposals", cabalID: publicCabalID, actor: 5, symbol: "STRIPEx",
            title: "Rosa proposed buying $300 of STRIPEx", detail: "Night Shift", status: "open", comments: 4,
            vote: Vote(yes: 2, no: 1, voters: 9, needed: 5, endsInHours: 18), minutesAgo: 50),
    ]

    private static let publicCabalID = "00000000-0000-7000-8000-0000000fc002"

    private struct Vote {
        let yes: Int, no: Int, voters: Int, needed: Int, endsInHours: Double
    }

    private static let actorNames = ["Maya", "Alex", "Jordan", "Sam", "Rosa"]

    private static func sample(
        _ number: Int, kind: String, refType: String, cabalID: String?, actor: Int?, symbol: String?,
        title: String, detail: String?, body: String? = nil, status: String? = nil, tone: String = "neutral",
        comments: Int32 = 0, reactions: [Components.Schemas.Reaction] = [], vote: Vote? = nil,
        minutesAgo: Double
    ) -> Self {
        let created = Date(timeIntervalSinceNow: -minutesAgo * 60)
        return Self(
            id: String(format: "00000000-0000-7000-8000-0000000fe%03d", number), kind: kind, refType: refType,
            refId: String(format: "00000000-0000-7000-8000-0000000fa%03d", number), cabalId: cabalID,
            cabalName: cabalID.map { $0 == publicCabalID ? (detail ?? "") : "Weekend investors" },
            actorId: actor.map { String(format: "00000000-0000-7000-8000-0000000fb%03d", $0) },
            actorName: actor.map { actorNames[$0 - 1] },
            actorPhotoUrl: nil,
            assetId: symbol.map { _ in String(format: "00000000-0000-7000-8000-0000000fd%03d", number) },
            symbol: symbol,
            title: title, detail: detail, body: body, status: status, tone: tone, commentCount: comments,
            likeCount: 0, likedByMe: false, reactions: reactions,
            reactionCount: reactions.reduce(0) { $0 + $1.count },
            tally: vote.map { .init(yes: $0.yes, no: $0.no, voters: $0.voters, needed: $0.needed) },
            expiresAt: vote.map { Date(timeIntervalSinceNow: $0.endsInHours * 3600) },
            returnPrivate: false,
            createdAt: created, updatedAt: created)
    }
}
#endif
