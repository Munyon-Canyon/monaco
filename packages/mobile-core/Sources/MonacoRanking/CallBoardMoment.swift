import Foundation
import MonacoAPI

public struct CallBoardMoment: Equatable, Sendable {
    public let id: String
    public let seenKey: String
    public let leaderID: String
    public let leaderName: String
    public let leaderHandle: String?
    public let leaderPictureURL: String?
    public let leaderIsCabal: Bool
    public let title: String
    public let subline: String
    public let glyph: String?
    public let spoken: String

    init(_ change: Components.Schemas.CallBoard.LeadChangePayload, seenKey: String) {
        let isCabal = change.leader.kind == .cabal
        let call = change.call.map { (kind: $0.kind == .buy ? "Buy" : "Sell", symbol: $0.symbol) }
        let pct = Int(change.leaderRatePct)
        id = change.id
        self.seenKey = seenKey
        leaderID = change.leader.id
        leaderName = change.leader.name
        leaderHandle = change.leader.handle
        leaderPictureURL = change.leader.pictureUrl
        leaderIsCabal = isCabal
        title = CallBoardCopy.leadTitle(leader: change.leader.name, isCabal: isCabal)
        subline = CallBoardCopy.leadSubline(passed: change.passed.name, isCabal: isCabal, call: call, ratePct: pct)
        glyph = CallBoardCopy.leadGlyph(places: Int(change.movementPlaces))
        spoken = CallBoardCopy.leadSpoken(
            leader: change.leader.name, passed: change.passed.name, isCabal: isCabal, call: call, ratePct: pct)
    }
}

public protocol CallBoardSeenBacking {
    func read(_ key: String) throws -> String?
    func write(_ key: String, _ value: String)
}

extension UserDefaults: CallBoardSeenBacking {
    public func read(_ key: String) throws -> String? { string(forKey: key) }
    public func write(_ key: String, _ value: String) { set(value, forKey: key) }
}

public struct CallBoardSeenStore {
    private let backing: any CallBoardSeenBacking

    public init(backing: any CallBoardSeenBacking = UserDefaults.standard) { self.backing = backing }

    private func key(_ moment: CallBoardMoment) -> String { "callboard.leadchange.\(moment.seenKey)" }

    public func shouldShow(_ moment: CallBoardMoment) -> Bool {
        guard let last = try? backing.read(key(moment)) else { return true }
        return last != moment.id
    }

    public func markSeen(_ moment: CallBoardMoment) { backing.write(key(moment), moment.id) }
}
