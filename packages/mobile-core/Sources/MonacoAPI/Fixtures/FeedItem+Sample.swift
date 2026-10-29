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
            vote: Vote(yes: 4, no: 1, voters: 8, needed: 5, endsInHours: 4), thread: .members("Weekend investors"),
            minutesAgo: 4),
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

    public static let threads: [Self] = [
        sample(
            12, kind: "proposal", refType: "proposals", cabalID: sampleCabalID, actor: 2, symbol: "NVDAx",
            title: "Alex proposed buying $400 of NVDAx", detail: "Weekend investors", status: "open", comments: 6,
            vote: Vote(yes: 3, no: 1, voters: 8, needed: 5, endsInHours: 9), thread: .members("Weekend investors"),
            minutesAgo: 8),
        sample(
            13, kind: "reply", refType: "feed_comments", cabalID: sampleCabalID, actor: 5, symbol: "TSLAx",
            title: "Rosa replied to Logan's proposal to sell TSLAx", detail: "Weekend investors",
            body: "Selling into a 6% drop feels backwards.",
            thread: .members("Weekend investors", of: 19, count: 4), minutesAgo: 20),
    ]

    public static let publicProposals: [Self] = [
        sample(
            6, kind: "proposal", refType: "proposals", cabalID: publicCabalID, actor: 1, symbol: "AMZNx",
            title: "Maya proposed buying $250 of AMZNx", detail: "Pilates", status: "open", comments: 2,
            vote: Vote(yes: 3, no: 0, voters: 7, needed: 4, endsInHours: 23),
            thread: .members("Pilates", canReply: false), minutesAgo: 30),
        sample(
            7, kind: "proposal", refType: "proposals", cabalID: publicCabalID, actor: 5, symbol: "STRIPEx",
            title: "Rosa proposed buying $300 of STRIPEx", detail: "Night Shift", status: "open", comments: 4,
            vote: Vote(yes: 2, no: 1, voters: 9, needed: 5, endsInHours: 18),
            thread: .members("Night Shift", canReply: false), minutesAgo: 50),
    ]

    public static let results: [Self] = [
        result(
            ResultFacts(
                number: 8, cabal: "Degens", actor: 6, symbol: "METAx", assetName: "META", action: "buy",
                usdcMicros: 300_000_000, title: "Degens passed Buy META $300 4–1", detail: "buying",
                vote: Vote(yes: 4, no: 1, voters: 6, needed: 4, endsInHours: 0), outcome: .passed, shape: .cabal,
                assetKind: .equity, called: true, minutesAgo: 300,
                thread: .members("Degens", of: 18, count: 5))),
        result(
            ResultFacts(
                number: 9, cabal: "Tech Titans", actor: 7, symbol: "AAPLx", assetName: "AAPL", action: "sell",
                usdcMicros: 480_000_000, title: "Logan's sale of $480 of AAPL in Tech Titans passed 3–1",
                detail: "selling", vote: Vote(yes: 3, no: 1, voters: 4, needed: 3, endsInHours: 0), outcome: .passed,
                shape: .member, assetKind: .equity, called: true, minutesAgo: 300, thread: .anyone(count: 5))),
        result(
            ResultFacts(
                number: 10, cabal: "Quant Kids", actor: 4, symbol: "OPENAI", assetName: "OpenAI", action: "buy",
                usdcMicros: 200_000_000, title: "Sam's buy of $200 of OpenAI in Quant Kids passed",
                detail: "buying", vote: Vote(yes: 3, no: 0, voters: 5, needed: 3, endsInHours: 0), outcome: .passed,
                shape: .member, assetKind: .preIpo, called: true, minutesAgo: 60 * 24 * 4)),
        result(
            ResultFacts(
                number: 11, cabal: "Tech Titans", actor: 7, symbol: "TSLAx", assetName: "TSLA", action: "sell",
                usdcMicros: 900_000_000, title: "Tech Titans failed Sell TSLA $900 2–2", detail: "no trade",
                vote: Vote(yes: 2, no: 2, voters: 5, needed: 3, endsInHours: 0), outcome: .failed, shape: .cabal,
                assetKind: .equity, called: false, minutesAgo: 60 * 24 * 4,
                thread: .members("Tech Titans", of: 19, count: 4))),
    ]

    private static let publicCabalID = "00000000-0000-7000-8000-0000000fc002"

    private struct ResultFacts {
        let number: Int, cabal: String, actor: Int, symbol: String, assetName: String, action: String
        let usdcMicros: Int64, title: String, detail: String, vote: Vote
        let outcome: Components.Schemas.FeedItem.OutcomePayload
        let shape: Components.Schemas.FeedItem.ShapePayload
        let assetKind: Components.Schemas.FeedItem.AssetKindPayload
        let called: Bool, minutesAgo: Double
        var thread: Thread?
    }

    private static let resultActorNames = [4: "Sam", 6: "Christelle", 7: "Logan"]

    private static func result(_ facts: ResultFacts) -> Self {
        let created = Date(timeIntervalSinceNow: -facts.minutesAgo * 60)
        let actorID = String(format: "00000000-0000-7000-8000-0000000fb%03d", facts.actor)
        let name = resultActorNames[facts.actor] ?? ""
        let fields = threadFields(facts.thread, number: facts.number, comments: 0)
        return Self(
            id: String(format: "00000000-0000-7000-8000-0000000fe%03d", facts.number), kind: "result",
            refType: "proposals", refId: String(format: "00000000-0000-7000-8000-0000000fa%03d", facts.number),
            cabalId: String(format: "00000000-0000-7000-8000-0000000fc%03d", 10 + facts.number),
            cabalName: facts.cabal, actorId: actorID, actorName: name, actorPhotoUrl: nil,
            assetId: String(format: "00000000-0000-7000-8000-0000000fd%03d", facts.number), symbol: facts.symbol,
            title: facts.title, detail: facts.detail, status: facts.outcome == .passed ? "passed" : "failed",
            tone: "neutral", action: facts.action, usdcMicros: facts.usdcMicros, assetName: facts.assetName,
            proposalId: String(format: "00000000-0000-7000-8000-0000000fa%03d", facts.number), commentCount: 0,
            threadItemId: fields.threadItemId, replyCount: fields.replyCount, canReply: fields.canReply,
            canReact: fields.canReact, replyRule: fields.replyRule, replyRuleCabalName: fields.replyRuleCabalName,
            likeCount: 0, likedByMe: false, reactions: [], reactionCount: 0,
            tally: .init(
                yes: facts.vote.yes, no: facts.vote.no, voters: facts.vote.voters, needed: facts.vote.needed),
            outcome: facts.outcome, shape: facts.shape,
            caller: facts.called ? .init(id: actorID, name: name, photoUrl: nil) : nil,
            assetKind: facts.assetKind, returnPrivate: false, createdAt: created, updatedAt: created)
    }

    private struct Thread {
        let itemNumber: Int?, count: Int32?, members: Bool, canReply: Bool, cabalName: String?

        static func anyone(count: Int32) -> Thread {
            Thread(itemNumber: nil, count: count, members: false, canReply: true, cabalName: nil)
        }

        static func members(_ cabal: String, of itemNumber: Int? = nil, count: Int32? = nil, canReply: Bool = true)
            -> Thread
        {
            Thread(itemNumber: itemNumber, count: count, members: true, canReply: canReply, cabalName: cabal)
        }
    }

    private static func itemID(_ number: Int) -> String {
        String(format: "00000000-0000-7000-8000-0000000fe%03d", number)
    }

    private static func threadFields(_ thread: Thread?, number: Int, comments: Int32) -> ThreadFields {
        let own = thread ?? Thread(itemNumber: nil, count: nil, members: false, canReply: true, cabalName: nil)
        return ThreadFields(
            threadItemId: itemID(own.itemNumber ?? number), replyCount: own.count ?? comments,
            canReply: own.canReply, canReact: true, replyRule: own.members ? .members : .anyone,
            replyRuleCabalName: own.members ? own.cabalName : nil)
    }

    private struct ThreadFields {
        let threadItemId: String, replyCount: Int32, canReply: Bool, canReact: Bool
        let replyRule: Components.Schemas.FeedItem.ReplyRulePayload, replyRuleCabalName: String?
    }

    private struct Vote {
        let yes: Int, no: Int, voters: Int, needed: Int, endsInHours: Double
    }

    private static let actorNames = ["Maya", "Alex", "Jordan", "Sam", "Rosa"]

    private static func sample(
        _ number: Int, kind: String, refType: String, cabalID: String?, actor: Int?, symbol: String?,
        title: String, detail: String?, body: String? = nil, status: String? = nil, tone: String = "neutral",
        comments: Int32 = 0, reactions: [Components.Schemas.Reaction] = [], vote: Vote? = nil,
        thread: Thread? = nil, minutesAgo: Double
    ) -> Self {
        let created = Date(timeIntervalSinceNow: -minutesAgo * 60)
        let fields = threadFields(thread, number: number, comments: comments)
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
            threadItemId: fields.threadItemId, replyCount: fields.replyCount, canReply: fields.canReply,
            canReact: fields.canReact, replyRule: fields.replyRule, replyRuleCabalName: fields.replyRuleCabalName,
            likeCount: 0, likedByMe: false, reactions: reactions,
            reactionCount: reactions.reduce(0) { $0 + $1.count },
            tally: vote.map { .init(yes: $0.yes, no: $0.no, voters: $0.voters, needed: $0.needed) },
            expiresAt: vote.map { Date(timeIntervalSinceNow: $0.endsInHours * 3600) },
            returnPrivate: false,
            createdAt: created, updatedAt: created)
    }
}
#endif
