#if DEBUG
import Foundation

extension Components.Schemas.CallBoard {
    private typealias Row = Components.Schemas.CallBoardRow

    private static func row(
        _ rank: Int32, _ name: String, _ pct: Int32, right: Int32, resolved: Int32, voted: Int32, eligible: Int32,
        up: Int32 = 0, viewer: Bool = false, last: (String, Bool)? = nil
    ) -> Row {
        let kind: Components.Schemas.CallBoardMovement.KindPayload = up > 0 ? .up : (up < 0 ? .down : .same)
        return Row(
            rank: rank,
            subject: .init(
                id: "00000000-0000-7000-8000-0000000000\(String(format: "%02d", rank))", kind: .user, name: name,
                handle: name.lowercased(), pictureUrl: nil),
            rateBps: pct * 100, ratePct: pct, right: right, resolved: resolved, voted: voted, eligible: eligible,
            turnoutBps: voted * 10_000 / eligible, movement: .init(kind: kind, places: abs(up)),
            lastCall: last.map {
                .init(proposalId: "00000000-0000-7000-8000-00000000b001", kind: .buy, symbol: $0.0, right: $0.1)
            },
            isViewer: viewer, isMine: false)
    }

    private static let christelle = row(1, "Christelle", 92, right: 12, resolved: 13, voted: 14, eligible: 14, up: 1)
    private static let logan = row(2, "Logan", 91, right: 10, resolved: 11, voted: 12, eligible: 12, up: -1)
    private static let mia = row(3, "Mia", 89, right: 8, resolved: 9, voted: 10, eligible: 11)
    private static let sam = row(6, "Sam", 83, right: 5, resolved: 6, voted: 8, eligible: 9)
    private static let kai = row(7, "Kai", 79, right: 11, resolved: 14, voted: 14, eligible: 15)
    private static let andres = row(
        8, "Andres", 78, right: 7, resolved: 9, voted: 11, eligible: 11, up: 1, viewer: true, last: ("NVDA", true))
    private static let rosa = row(
        9, "Rosa", 75, right: 6, resolved: 8, voted: 9, eligible: 10, up: -1, last: ("TSLA", false))
    private static let nora = row(10, "Nora", 71, right: 5, resolved: 7, voted: 8, eligible: 10)
    private static let diego = row(11, "Diego", 67, right: 4, resolved: 6, voted: 7, eligible: 9)
    private static let maya = row(12, "Maya", 60, right: 3, resolved: 5, voted: 6, eligible: 9)

    public static let peopleMonth = Self(
        board: .people, filter: .all,
        period: .init(kind: .month, start: "2026-10-01", end: "2026-10-31"),
        computedAt: Date(timeIntervalSince1970: 1_791_766_800), totalRanked: 24, top: [christelle, logan, mia],
        me: .init(andres), above: .init(kai), below: .init(rosa),
        gap: .init(versus: .above, points: -1, callsToPass: 1, aboveIsPerfect: false), climbBps: 7_777,
        around: .init(rows: [sam, kai, andres, rosa, nora, diego, maya]), unranked: [])

    public static let peopleMonthLeadChange: Self = {
        var board = peopleMonth
        board.around = .init(rows: [sam, kai])
        board.leadChange = .init(
            id: "00000000-0000-7000-8000-00000000c001", leader: christelle.subject, passed: logan.subject,
            leaderRatePct: 92,
            call: .init(proposalId: "00000000-0000-7000-8000-00000000b001", kind: .buy, symbol: "META"),
            changedAt: Date(timeIntervalSince1970: 1_791_766_800), movementPlaces: 1)
        return board
    }()

    public static let peopleMonthLeadChangeNoCall: Self = {
        var board = peopleMonthLeadChange
        board.leadChange?.id = "00000000-0000-7000-8000-00000000c002"
        board.leadChange?.call = nil
        return board
    }()

    public static let empty = Self(
        board: .people, filter: .all, period: .init(kind: .month, start: "2026-10-01", end: "2026-10-31"),
        computedAt: Date(timeIntervalSince1970: 1_791_766_800), totalRanked: 0, top: [], climbBps: 0,
        around: .init(rows: []), unranked: [])

    public static let allTimeLeader = Self(
        board: .people, filter: .friends, period: .init(kind: .all),
        computedAt: Date(timeIntervalSince1970: 1_791_766_800),
        totalRanked: 10, top: [christelle], me: .init(christelle), below: .init(logan),
        gap: .init(versus: .below, points: 1, callsToPass: nil, aboveIsPerfect: false), climbBps: 9_200,
        around: .init(rows: [christelle]), unranked: [])
}

extension Components.Schemas.CallBoard.MePayload {
    fileprivate init(_ r: Components.Schemas.CallBoardRow) {
        self.init(
            rank: r.rank, subject: r.subject, rateBps: r.rateBps, ratePct: r.ratePct, right: r.right,
            resolved: r.resolved, voted: r.voted, eligible: r.eligible, turnoutBps: r.turnoutBps, needed: r.needed,
            movement: r.movement,
            lastCall: r.lastCall.map {
                .init(proposalId: $0.proposalId, kind: .buy, symbol: $0.symbol, right: $0.right)
            },
            isViewer: r.isViewer, isMine: r.isMine)
    }
}

extension Components.Schemas.CallBoard.AbovePayload {
    fileprivate init(_ r: Components.Schemas.CallBoardRow) {
        self.init(
            rank: r.rank, subject: r.subject, rateBps: r.rateBps, ratePct: r.ratePct, right: r.right,
            resolved: r.resolved, voted: r.voted, eligible: r.eligible, turnoutBps: r.turnoutBps, needed: r.needed,
            movement: r.movement,
            lastCall: r.lastCall.map {
                .init(proposalId: $0.proposalId, kind: .buy, symbol: $0.symbol, right: $0.right)
            },
            isViewer: r.isViewer, isMine: r.isMine)
    }
}

extension Components.Schemas.CallBoard.BelowPayload {
    fileprivate init(_ r: Components.Schemas.CallBoardRow) {
        self.init(
            rank: r.rank, subject: r.subject, rateBps: r.rateBps, ratePct: r.ratePct, right: r.right,
            resolved: r.resolved, voted: r.voted, eligible: r.eligible, turnoutBps: r.turnoutBps, needed: r.needed,
            movement: r.movement,
            lastCall: r.lastCall.map {
                .init(proposalId: $0.proposalId, kind: .buy, symbol: $0.symbol, right: $0.right)
            },
            isViewer: r.isViewer, isMine: r.isMine)
    }
}
#endif
