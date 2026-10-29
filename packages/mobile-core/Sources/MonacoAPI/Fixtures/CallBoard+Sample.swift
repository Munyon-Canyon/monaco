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

    private static func entry(
        _ id: Int, _ kind: Components.Schemas.LeaderboardSubject.KindPayload, _ name: String, rank: Int32?,
        pct: Int32?, right: Int32, resolved: Int32, voted: Int32, eligible: Int32, turnout: Int32, up: Int32 = 0,
        needed: Int32? = nil, viewer: Bool = false, mine: Bool = false
    ) -> Row {
        let move: Components.Schemas.CallBoardMovement.KindPayload = up > 0 ? .up : (up < 0 ? .down : .same)
        return Row(
            rank: rank,
            subject: .init(
                id: "00000000-0000-7000-8000-0000000001\(String(format: "%02d", id))", kind: kind, name: name,
                handle: kind == .user ? name.lowercased() : nil, pictureUrl: nil),
            rateBps: pct.map { $0 * 100 }, ratePct: pct, right: right, resolved: resolved, voted: voted,
            eligible: eligible, turnoutBps: turnout * 100, needed: needed,
            movement: .init(kind: move, places: abs(up)), lastCall: nil, isViewer: viewer, isMine: mine)
    }

    private static let pilates = entry(
        1, .cabal, "Pilates", rank: 1, pct: 93, right: 13, resolved: 14, voted: 14, eligible: 14, turnout: 96, up: 1)
    private static let techTitans = entry(
        2, .cabal, "Tech Titans", rank: 2, pct: 89, right: 8, resolved: 9, voted: 12, eligible: 13, turnout: 92,
        up: -1, mine: true)
    private static let acCapital = entry(
        3, .cabal, "AC Capital", rank: 3, pct: 86, right: 6, resolved: 7, voted: 15, eligible: 16, turnout: 94,
        mine: true)
    private static let degens = entry(
        4, .cabal, "Degens", rank: 4, pct: 83, right: 10, resolved: 12, voted: 14, eligible: 16, turnout: 88, up: 2,
        mine: true)
    private static let quantKids = entry(
        5, .cabal, "Quant Kids", rank: 5, pct: 80, right: 4, resolved: 5, voted: 6, eligible: 7, turnout: 85)
    private static let nightShift = entry(
        6, .cabal, "Night Shift", rank: 6, pct: 75, right: 6, resolved: 8, voted: 9, eligible: 11, turnout: 81)

    private static let weekChristelle = entry(
        21, .user, "Christelle", rank: 1, pct: 100, right: 6, resolved: 6, voted: 6, eligible: 6, turnout: 100)
    private static let weekLogan = entry(
        22, .user, "Logan", rank: 2, pct: 86, right: 6, resolved: 7, voted: 7, eligible: 7, turnout: 100, up: 1)
    private static let weekMia = entry(
        23, .user, "Mia", rank: 3, pct: 83, right: 5, resolved: 6, voted: 6, eligible: 7, turnout: 86, up: -1)
    private static let weekKai = entry(
        24, .user, "Kai", rank: 4, pct: 80, right: 4, resolved: 5, voted: 6, eligible: 7, turnout: 86, up: 2)
    private static let weekJordan = entry(
        25, .user, "Jordan", rank: 5, pct: 80, right: 4, resolved: 5, voted: 5, eligible: 6, turnout: 83, up: -1)
    private static let weekAndres = entry(
        26, .user, "Andres", rank: nil, pct: nil, right: 2, resolved: 3, voted: 5, eligible: 6, turnout: 83,
        needed: 2, viewer: true)
    private static let weekRosa = entry(
        27, .user, "Rosa", rank: nil, pct: nil, right: 1, resolved: 2, voted: 3, eligible: 4, turnout: 75, needed: 3)

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

    private static let friendJordan = row(4, "Jordan", 88, right: 7, resolved: 8, voted: 9, eligible: 9)
    private static let friendKai = row(5, "Kai", 85, right: 11, resolved: 13, voted: 14, eligible: 15)
    private static let friendAndres = row(
        6, "Andres", 78, right: 7, resolved: 9, voted: 11, eligible: 11, up: 1, viewer: true, last: ("NVDA", true))
    private static let friendRosa = row(
        7, "Rosa", 75, right: 6, resolved: 8, voted: 9, eligible: 10, up: -1, last: ("TSLA", false))
    private static let friendNora = row(8, "Nora", 71, right: 5, resolved: 7, voted: 8, eligible: 10)
    private static let friendDiego = row(9, "Diego", 67, right: 4, resolved: 6, voted: 7, eligible: 9)
    private static let friendMaya = row(10, "Maya", 60, right: 3, resolved: 5, voted: 6, eligible: 9)

    public static let peopleMonth = Self(
        board: .people, filter: .all,
        period: .init(kind: .month, start: "2026-10-01", end: "2026-10-31"),
        computedAt: Date(timeIntervalSince1970: 1_791_766_800), totalRanked: 24, top: [christelle, logan, mia],
        me: .init(andres), above: .init(kai), below: .init(rosa),
        gap: .init(versus: .above, points: -1, callsToPass: 1, aboveIsPerfect: false), climbBps: 7_777,
        around: .init(rows: [sam, kai, andres, rosa, nora, diego, maya], afterCursor: "cursor-page-2"), unranked: [])

    public static let peopleMonthPage2 = Self(
        board: .people, filter: .all,
        period: .init(kind: .month, start: "2026-10-01", end: "2026-10-31"),
        computedAt: Date(timeIntervalSince1970: 1_791_766_800), totalRanked: 24, top: [], climbBps: 0,
        around: .init(
            rows: (13...24).map {
                row(Int32($0), "Member \($0)", Int32(60 - ($0 - 13)), right: 3, resolved: 5, voted: 5, eligible: 6)
            }, afterCursor: nil),
        unranked: [])

    public static let friendsMonth = Self(
        board: .people, filter: .friends,
        period: .init(kind: .month, start: "2026-10-01", end: "2026-10-31"),
        computedAt: Date(timeIntervalSince1970: 1_791_766_800), totalRanked: 10, top: [christelle, logan, mia],
        me: .init(friendAndres), above: .init(friendKai), below: .init(friendRosa),
        gap: .init(versus: .above, points: -7, callsToPass: 1, aboveIsPerfect: false), climbBps: 7_800,
        around: .init(
            rows: [friendJordan, friendKai, friendAndres, friendRosa, friendNora, friendDiego, friendMaya],
            afterCursor: nil),
        unranked: [])

    public static let peopleMonthLeadChange: Self = {
        var board = peopleMonth
        var slipped = sam
        slipped.movement = .init(kind: .down, places: 1)
        board.around = .init(rows: [slipped, kai])
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

    public static let cabalsMonth = Self(
        board: .cabals, filter: .all, period: .init(kind: .month, start: "2026-10-01", end: "2026-10-31"),
        computedAt: Date(timeIntervalSince1970: 1_791_766_800), totalRanked: 8, top: [pilates, techTitans, acCapital],
        me: .init(techTitans), above: .init(pilates),
        gap: .init(versus: .above, points: -4, callsToPass: nil, aboveIsPerfect: false), climbBps: 8_900,
        around: .init(rows: [techTitans, acCapital, degens, quantKids, nightShift]), unranked: [])

    public static let cabalsUnranked = Self(
        board: .cabals, filter: .all, period: .init(kind: .month, start: "2026-10-01", end: "2026-10-31"),
        computedAt: Date(timeIntervalSince1970: 1_791_766_800), totalRanked: 8, top: [pilates, techTitans, acCapital],
        climbBps: 6_000,
        unrankedMe: .init(
            subject: .init(
                id: "00000000-0000-7000-8000-000000000199", kind: .cabal, name: "Tech Titans", handle: nil,
                pictureUrl: nil),
            resolved: 3, needed: 2, voted: 5, eligible: 6, pendingSymbols: ["SpaceX"]),
        around: .init(rows: [degens, quantKids]), unranked: [])

    public static let peopleWeekUnranked = Self(
        board: .people, filter: .all, period: .init(kind: .week, start: "2026-10-19", end: "2026-10-25"),
        computedAt: Date(timeIntervalSince1970: 1_791_766_800), totalRanked: 5,
        top: [weekChristelle, weekLogan, weekMia], climbBps: 6_000,
        unrankedMe: .init(
            subject: weekAndres.subject, resolved: 3, needed: 2, voted: 5, eligible: 6,
            pendingSymbols: ["SpaceX", "AMZN"]),
        around: .init(rows: [weekKai, weekJordan]), unranked: [weekAndres, weekRosa])

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
