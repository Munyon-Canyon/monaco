import Foundation
import MonacoAPI

public enum CallBoardKind: String, Equatable, Sendable { case people, cabals }

public enum CallFilter: String, Equatable, Sendable { case all, friends }

public enum CallPeriod: String, CaseIterable, Equatable, Sendable { case week, month, all }

public enum CallBoardTab: String, CaseIterable, Equatable, Sendable {
    case people, cabals, friends

    public var board: CallBoardKind { self == .cabals ? .cabals : .people }
    public var filter: CallFilter { self == .friends ? .friends : .all }

    public init(board: CallBoardKind, filter: CallFilter) {
        self = board == .cabals ? .cabals : (filter == .friends ? .friends : .people)
    }
}

public enum CallBoardMovement: Equatable, Sendable {
    case up(Int)
    case down(Int)
    case same
    case new

    init(_ movement: Components.Schemas.CallBoardMovement) {
        switch movement.kind {
        case .up: self = .up(Int(movement.places))
        case .down: self = .down(Int(movement.places))
        case .same: self = .same
        case .new: self = .new
        }
    }

    public var signedPlaces: Int? {
        switch self {
        case .up(let places): places
        case .down(let places): -places
        case .same, .new: nil
        }
    }
}

public struct CallBoardRowValue: Equatable, Sendable, Identifiable {
    public let id: String
    public let isCabal: Bool
    public let rank: Int?
    public let name: String
    public let handle: String?
    public let pictureURL: String?
    public let isViewer: Bool
    public let isMine: Bool
    public let rate: String
    public let record: String
    public let turnout: String
    public let movement: CallBoardMovement
    public let movementGlyph: String
    public let movementSpoken: String?
    public let spoken: String

    init(
        subject: Components.Schemas.LeaderboardSubject, rank: Int32?, ratePct: Int32?, right: Int32, resolved: Int32,
        voted: Int32, eligible: Int32, movement: Components.Schemas.CallBoardMovement,
        lastCall: (symbol: String, right: Bool)?, isViewer: Bool, isMine: Bool
    ) {
        let move = CallBoardMovement(movement)
        let pct = ratePct.map(Int.init)
        id = subject.id
        isCabal = subject.kind == .cabal
        self.rank = rank.map(Int.init)
        name = subject.name
        handle = subject.handle
        pictureURL = subject.pictureUrl
        self.isViewer = isViewer
        self.isMine = isMine
        rate = CallBoardCopy.rate(pct)
        record = CallBoardCopy.record(right: Int(right), resolved: Int(resolved))
        turnout = CallBoardCopy.turnout(voted: Int(voted), eligible: Int(eligible))
        self.movement = move
        movementGlyph = CallBoardCopy.movementGlyph(move)
        movementSpoken = CallBoardCopy.movementSpoken(move, lastCall: lastCall)
        spoken = CallBoardCopy.spoken(
            rank: self.rank, name: subject.name, isViewer: isViewer, ratePct: pct, right: Int(right),
            resolved: Int(resolved), voted: Int(voted), eligible: Int(eligible), movement: move)
    }

    init(_ row: Components.Schemas.CallBoardRow) {
        self.init(
            subject: row.subject, rank: row.rank, ratePct: row.ratePct, right: row.right, resolved: row.resolved,
            voted: row.voted, eligible: row.eligible, movement: row.movement,
            lastCall: row.lastCall.map { ($0.symbol, $0.right) }, isViewer: row.isViewer, isMine: row.isMine)
    }

    init(_ row: Components.Schemas.CallBoard.MePayload) {
        self.init(
            subject: row.subject, rank: row.rank, ratePct: row.ratePct, right: row.right, resolved: row.resolved,
            voted: row.voted, eligible: row.eligible, movement: row.movement,
            lastCall: row.lastCall.map { ($0.symbol, $0.right) }, isViewer: row.isViewer, isMine: row.isMine)
    }

    init(_ row: Components.Schemas.CallBoard.AbovePayload) {
        self.init(
            subject: row.subject, rank: row.rank, ratePct: row.ratePct, right: row.right, resolved: row.resolved,
            voted: row.voted, eligible: row.eligible, movement: row.movement,
            lastCall: row.lastCall.map { ($0.symbol, $0.right) }, isViewer: row.isViewer, isMine: row.isMine)
    }

    init(_ row: Components.Schemas.CallBoard.BelowPayload) {
        self.init(
            subject: row.subject, rank: row.rank, ratePct: row.ratePct, right: row.right, resolved: row.resolved,
            voted: row.voted, eligible: row.eligible, movement: row.movement,
            lastCall: row.lastCall.map { ($0.symbol, $0.right) }, isViewer: row.isViewer, isMine: row.isMine)
    }
}

public struct CallBoardCard: Equatable, Sendable {
    public let me: CallBoardRowValue
    public let neighbour: CallBoardRowValue?
    public let rankText: String
    public let ofLine: String
    public let neighbourLine: String?
    public let gapLine: String?
    public let climb: Double
    public let standing: String?
}

public struct CallBoardScreen: Equatable, Sendable {
    public let period: CallPeriod
    public let periodTitle: String
    public let periodRange: String?
    public let card: CallBoardCard?
    public let top: [CallBoardRowValue]
    public let around: [CallBoardRowValue]
    public let emptyMessage: String
    public let moment: CallBoardMoment?
    private let viewerListed: Bool

    public func showsDocked(viewerRowVisible: Bool) -> Bool {
        guard card != nil else { return false }
        return viewerListed ? !viewerRowVisible : true
    }

    public var isEmpty: Bool { top.isEmpty && around.isEmpty && card == nil }

    public init(_ board: Components.Schemas.CallBoard) {
        let period = CallPeriod(rawValue: board.period.kind.rawValue) ?? .month
        let tab = CallBoardTab(
            board: board.board == .cabals ? .cabals : .people, filter: board.filter == .friends ? .friends : .all)
        self.period = period
        periodTitle = CallBoardCopy.periodTitle(period)
        periodRange = CallBoardCopy.periodRange(board.period)
        top = board.top.map(CallBoardRowValue.init)
        around = board.around.rows.map(CallBoardRowValue.init)
        emptyMessage = CallBoardCopy.emptyMessage(period)
        moment = board.leadChange.map {
            CallBoardMoment($0, seenKey: "\(board.board.rawValue).\(board.filter.rawValue).\(period.rawValue)")
        }
        let listed = board.top + board.around.rows
        viewerListed = board.me.map { me in listed.contains { $0.subject.id == me.subject.id } } ?? false
        card = board.me.map { me in
            let mine = CallBoardRowValue(me)
            let above = board.above.map(CallBoardRowValue.init)
            let below = board.below.map(CallBoardRowValue.init)
            let neighbour = board.gap?.versus == .below ? below : above
            let shortName = neighbour.map { $0.isCabal ? $0.name : String($0.name.split(separator: " ").first ?? "") }
            return CallBoardCard(
                me: mine, neighbour: neighbour, rankText: mine.rank.map(CallBoardCopy.rankText) ?? CallBoardCopy.dash,
                ofLine: CallBoardCopy.ofLine(total: Int(board.totalRanked), tab: tab),
                neighbourLine: neighbour.flatMap { row in
                    row.rank.flatMap { rank in shortName.map { CallBoardCopy.neighbour(name: $0, rank: rank) } }
                },
                gapLine: board.gap.flatMap { gap in
                    shortName.flatMap { CallBoardCopy.gapLine(gap, name: $0, period: period) }
                },
                climb: min(max(Double(board.climbBps) / 10_000, 0), 1),
                standing: mine.rank.map {
                    CallBoardCopy.standing(
                        rank: $0, total: Int(board.totalRanked), ratePct: me.ratePct.map(Int.init), period: period,
                        tab: tab)
                })
        }
    }
}
