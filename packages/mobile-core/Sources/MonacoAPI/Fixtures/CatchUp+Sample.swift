#if DEBUG
import Foundation

extension Components.Schemas.CatchUp {
    public static let sampleNow = Date(timeIntervalSince1970: 1_792_000_000)

    private static func person(_ n: Int, _ name: String) -> Components.Schemas.CatchUpPerson {
        .init(id: "00000000-0000-7000-8000-00000000a0\(n)", name: name, photoUrl: nil)
    }

    private static let acCapital = Components.Schemas.CatchUpCabal(
        id: "00000000-0000-7000-8000-00000000c001", name: "AC Capital", pictureUrl: nil)
    private static let techTitans = Components.Schemas.CatchUpCabal(
        id: "00000000-0000-7000-8000-00000000c002", name: "Tech Titans", pictureUrl: nil)
    private static let degens = Components.Schemas.CatchUpCabal(
        id: "00000000-0000-7000-8000-00000000c003", name: "Degens", pictureUrl: nil)
    private static let quantKids = Components.Schemas.CatchUpCabal(
        id: "00000000-0000-7000-8000-00000000c004", name: "Quant Kids", pictureUrl: nil)

    private static func proposal(
        _ n: Int, kind: String, symbol: String, name: String, usdc: Int64?, yes: Int, no: Int, voted: Int,
        eligible: Int, expiresIn: TimeInterval, outcome: Components.Schemas.CatchUpProposal.OutcomePayload? = nil,
        choice: Components.Schemas.CatchUpProposal.YourChoicePayload? = nil, filled: Bool? = nil
    ) -> Components.Schemas.CatchUpProposal {
        .init(
            id: "00000000-0000-7000-8000-00000000b00\(n)", kind: kind, symbol: symbol, assetName: name,
            assetLogoUrl: nil, usdcMicros: usdc, tokenAmount: nil, proposer: person(9, "Andres"),
            createdAt: sampleNow.addingTimeInterval(-86_400), expiresAt: sampleNow.addingTimeInterval(expiresIn),
            status: outcome == nil ? "open" : "closed", yes: yes, no: no, voted: voted, eligible: eligible,
            replyCount: 0, outcome: outcome, yourChoice: choice, filled: filled)
    }

    private static func item(
        _ key: String, _ kind: Components.Schemas.CatchUpItem.KindPayload, unread: Bool, ago: TimeInterval,
        actor: Components.Schemas.CatchUpPerson? = nil, cabal: Components.Schemas.CatchUpCabal,
        proposal: Components.Schemas.CatchUpProposal? = nil, quote: Components.Schemas.CatchUpQuote? = nil,
        invite: Components.Schemas.CatchUpInvite? = nil,
        reason: Components.Schemas.CatchUpItem.ClearedReasonPayload? = nil
    ) -> Components.Schemas.CatchUpItem {
        .init(
            key: key, kind: kind, unread: unread, at: sampleNow.addingTimeInterval(-ago), clearedReason: reason,
            actor: actor, cabal: cabal, proposal: proposal, quote: quote, invite: invite)
    }

    public static let six = Components.Schemas.CatchUp(
        count: 6, cabals: [acCapital, techTitans, degens],
        waiting: [
            item(
                "proposal:00000000-0000-7000-8000-00000000b001", .waitingVote, unread: true, ago: 3_600,
                cabal: acCapital,
                proposal: proposal(
                    1, kind: "buy", symbol: "NVDAx", name: "NVIDIA", usdc: 400_000_000, yes: 4, no: 1, voted: 5,
                    eligible: 8, expiresIn: 4 * 3_600))
        ],
        replies: [
            item(
                "comment:00000000-0000-7000-8000-00000000d001", .mention, unread: true, ago: 20 * 60,
                actor: person(1, "Christelle"), cabal: acCapital,
                proposal: proposal(
                    2, kind: "buy", symbol: "NVDAx", name: "NVIDIA", usdc: 400_000_000, yes: 4, no: 1, voted: 5,
                    eligible: 8, expiresIn: 4 * 3_600),
                quote: .init(authorName: "Christelle", isYou: false, text: "Still out: @Andres, Jordan?")),
            item(
                "comment:00000000-0000-7000-8000-00000000d002", .reply, unread: false, ago: 3 * 86_400,
                actor: person(2, "Mia"), cabal: techTitans,
                proposal: proposal(
                    3, kind: "sell", symbol: "TSLAx", name: "Tesla", usdc: nil, yes: 2, no: 2, voted: 4,
                    eligible: 4, expiresIn: -86_400, outcome: .failed, choice: .no),
                quote: .init(authorName: "You", isYou: true, text: "Deliveries miss is priced in.")),
        ],
        invites: [
            item(
                "invite:00000000-0000-7000-8000-00000000e001", .invite, unread: true, ago: 2 * 3_600,
                actor: person(3, "Sam"), cabal: quantKids,
                invite: .init(id: "00000000-0000-7000-8000-00000000e001", memberCount: 5, topHoldingName: "OpenAI"))
        ],
        results: [
            item(
                "proposal:00000000-0000-7000-8000-00000000b004", .result, unread: true, ago: 5 * 3_600, cabal: degens,
                proposal: proposal(
                    4, kind: "buy", symbol: "METAx", name: "Meta", usdc: 300_000_000, yes: 4, no: 1, voted: 5,
                    eligible: 5, expiresIn: -5 * 3_600, outcome: .filled, choice: .yes, filled: true)),
            item(
                "proposal:00000000-0000-7000-8000-00000000b005", .result, unread: false, ago: 4 * 86_400,
                cabal: techTitans,
                proposal: proposal(
                    5, kind: "sell", symbol: "TSLAx", name: "Tesla", usdc: nil, yes: 2, no: 2, voted: 4,
                    eligible: 4, expiresIn: -4 * 86_400, outcome: .failed, choice: .no)),
        ],
        cleared: [])

    public static let caughtUp = Components.Schemas.CatchUp(
        count: 0, cabals: [acCapital, techTitans, degens], waiting: [], replies: [], invites: [], results: [],
        cleared: [
            item(
                "proposal:00000000-0000-7000-8000-00000000b001", .result, unread: false, ago: 10, cabal: acCapital,
                proposal: proposal(
                    1, kind: "buy", symbol: "NVDAx", name: "NVIDIA", usdc: 400_000_000, yes: 5, no: 1, voted: 6,
                    eligible: 8, expiresIn: -10, outcome: .filled, choice: .yes, filled: true),
                reason: .voted),
            item(
                "comment:00000000-0000-7000-8000-00000000d001", .mention, unread: false, ago: 20 * 60,
                actor: person(1, "Christelle"), cabal: acCapital,
                proposal: proposal(
                    2, kind: "buy", symbol: "NVDAx", name: "NVIDIA", usdc: 400_000_000, yes: 4, no: 1, voted: 5,
                    eligible: 8, expiresIn: 4 * 3_600),
                quote: .init(authorName: "Christelle", isYou: false, text: "Still out: @Andres, Jordan?"),
                reason: .replied),
            item(
                "comment:00000000-0000-7000-8000-00000000d002", .reply, unread: false, ago: 3 * 86_400,
                actor: person(2, "Mia"), cabal: techTitans,
                proposal: proposal(
                    3, kind: "sell", symbol: "TSLAx", name: "Tesla", usdc: nil, yes: 2, no: 2, voted: 4,
                    eligible: 4, expiresIn: -86_400, outcome: .failed, choice: .no),
                quote: .init(authorName: "You", isYou: true, text: "Deliveries miss is priced in."),
                reason: .cleared),
            item(
                "invite:00000000-0000-7000-8000-00000000e001", .invite, unread: false, ago: 86_400,
                actor: person(3, "Sam"), cabal: quantKids,
                invite: .init(id: "00000000-0000-7000-8000-00000000e001", memberCount: 5, topHoldingName: nil),
                reason: .cleared),
            item(
                "proposal:00000000-0000-7000-8000-00000000b004", .result, unread: false, ago: 5 * 3_600,
                cabal: degens,
                proposal: proposal(
                    4, kind: "buy", symbol: "METAx", name: "Meta", usdc: 300_000_000, yes: 4, no: 1, voted: 5,
                    eligible: 5, expiresIn: -5 * 3_600, outcome: .filled, choice: .yes, filled: true),
                reason: .voted),
            item(
                "proposal:00000000-0000-7000-8000-00000000b005", .result, unread: false, ago: 4 * 86_400,
                cabal: techTitans,
                proposal: proposal(
                    5, kind: "sell", symbol: "TSLAx", name: "Tesla", usdc: nil, yes: 2, no: 2, voted: 4,
                    eligible: 4, expiresIn: -4 * 86_400, outcome: .failed, choice: .no),
                reason: .cleared),
        ])

    public static let firstUse = Components.Schemas.CatchUp(
        count: 0, cabals: [], waiting: [], replies: [], invites: [], results: [], cleared: [])
}
#endif
