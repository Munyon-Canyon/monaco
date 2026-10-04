#if DEBUG
import Foundation

extension Components.Schemas.FeedItem {
    public static let sampleCabalID = "00000000-0000-7000-8000-0000000fc001"

    public static let samples: [Self] = [
        sample(
            1, kind: "proposal", refType: "proposals", cabalID: sampleCabalID, actor: 1, symbol: "AAPLx",
            title: "Maya proposed buying $500 of AAPLx", detail: "Weekend investors",
            body: "Earnings beat three quarters running and services keep growing. A small slice before the call.",
            status: "open", minutesAgo: 4),
        sample(
            2, kind: "trade", refType: "swaps", cabalID: sampleCabalID, actor: 2, symbol: "NVDAx",
            title: "Weekend investors bought $250 of NVDAx", detail: "NVIDIA", tone: "positive", comments: 3,
            minutesAgo: 42),
        sample(
            3, kind: "price_move", refType: "assets", cabalID: nil, actor: nil, symbol: "TSLAx",
            title: "TSLAx fell 6.2% today", detail: "Tesla", tone: "negative", minutesAgo: 95),
        sample(
            4, kind: "cabal_created", refType: "cabals", cabalID: sampleCabalID, actor: 3, symbol: nil,
            title: "Jordan started Weekend investors", detail: "Open to anyone", minutesAgo: 60 * 5),
        sample(
            5, kind: "member_joined", refType: "cabal_members", cabalID: sampleCabalID, actor: 4, symbol: nil,
            title: "Sam joined Weekend investors", detail: nil, comments: 1, minutesAgo: 60 * 26),
    ]

    private static func sample(
        _ number: Int, kind: String, refType: String, cabalID: String?, actor: Int?, symbol: String?,
        title: String, detail: String?, body: String? = nil, status: String? = nil, tone: String = "neutral",
        comments: Int32 = 0, minutesAgo: Double
    ) -> Self {
        let created = Date(timeIntervalSinceNow: -minutesAgo * 60)
        return Self(
            id: String(format: "00000000-0000-7000-8000-0000000fe%03d", number), kind: kind, refType: refType,
            refId: String(format: "00000000-0000-7000-8000-0000000fa%03d", number), cabalId: cabalID,
            actorId: actor.map { String(format: "00000000-0000-7000-8000-0000000fb%03d", $0) }, symbol: symbol,
            title: title, detail: detail, body: body, status: status, tone: tone, commentCount: comments,
            createdAt: created, updatedAt: created)
    }
}
#endif
