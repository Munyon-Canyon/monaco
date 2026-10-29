import Foundation
import MonacoAPI

public enum CallBoardCopy {
    public static let title = "Leaderboard"
    public static let topThree = "Top three"
    public static let aroundYou = "Around you"
    public static let loadingLabel = "Loading the leaderboard"
    public static let errorTitle = "Can't load the leaderboard"
    public static let errorMessage = "Check your connection and try again."
    public static let tryAgain = "Try again"
    public static let emptyTitle = "Nobody ranked yet"
    public static let back = "Back"
    public static let infoLabel = "How the board is ranked"
    public static let dash = "–"

    public static func tabLabel(_ tab: CallBoardTab) -> String {
        switch tab {
        case .people: "People"
        case .cabals: "Cabals"
        case .friends: "Friends"
        }
    }

    public static func periodLabel(_ period: CallPeriod) -> String {
        switch period {
        case .week: "Week"
        case .month: "Month"
        case .all: "All time"
        }
    }

    public static func periodTitle(_ period: CallPeriod) -> String {
        switch period {
        case .week: "This week"
        case .month: "This month"
        case .all: "All time"
        }
    }

    public static func periodRange(_ period: Components.Schemas.CallBoardPeriod) -> String? {
        guard let start = period.start, let end = period.end else { return nil }
        return LedgerCopy.range(start: start, end: end)
    }

    public static func periodWord(_ period: CallPeriod) -> String {
        switch period {
        case .week: "this week"
        case .month: "this month"
        case .all: "yet"
        }
    }

    public static func emptyMessage(_ period: CallPeriod) -> String {
        switch period {
        case .all: "No one has five resolved votes yet."
        case .week, .month: "No one has five resolved votes yet \(periodWord(period))."
        }
    }

    public static func rankText(_ rank: Int) -> String { "#\(rank)" }

    public static func ofLine(total: Int, tab: CallBoardTab) -> String {
        switch tab {
        case .people: "of \(total) people"
        case .cabals: "of \(total) cabals"
        case .friends: "of \(total) friends"
        }
    }

    public static func neighbour(name: String, rank: Int) -> String { "\(name) is #\(rank)" }

    public static func gapLine(_ gap: Components.Schemas.CallBoard.GapPayload, name: String, period: CallPeriod)
        -> String?
    {
        switch gap.versus {
        case .above:
            if let calls = gap.callsToPass {
                return calls == 1
                    ? "One right call puts you past \(name)." : "\(calls) right calls put you past \(name)."
            }
            return gap.aboveIsPerfect ? "\(name) hasn't missed \(periodWord(period))." : nil
        case .below:
            let behind = abs(Int(gap.points))
            if behind == 0 { return "You lead. \(name) is level." }
            return "You lead. \(name) is \(behind) \(behind == 1 ? "point" : "points") behind."
        }
    }

    public static func rate(_ pct: Int?) -> String { pct.map { "\($0)%" } ?? dash }

    public static func record(right: Int, resolved: Int) -> String { "\(right) of \(resolved) right" }

    public static func turnout(voted: Int, eligible: Int) -> String { "Voted \(voted) of \(eligible)" }

    public static func movementGlyph(_ movement: CallBoardMovement) -> String {
        switch movement {
        case .up(let places): "▲ \(places)"
        case .down(let places): "▼ \(places)"
        case .same, .new: dash
        }
    }

    public static func movementSpoken(_ movement: CallBoardMovement, lastCall: (symbol: String, right: Bool)?)
        -> String?
    {
        let reason = lastCall.map { $0.right ? ", right on \($0.symbol)" : ", missed \($0.symbol)" } ?? ""
        switch movement {
        case .up(let places): return "Up \(places)\(reason)"
        case .down(let places): return "Down \(places)\(reason)"
        case .same, .new: return nil
        }
    }

    public static func spoken(
        rank: Int?, name: String, isViewer: Bool, ratePct: Int?, right: Int, resolved: Int, voted: Int, eligible: Int,
        movement: CallBoardMovement
    ) -> String {
        var parts = [rank.map(String.init) ?? "Not ranked", isViewer ? "\(name), you" : name]
        if let ratePct { parts.append("\(ratePct) percent") }
        parts.append(record(right: right, resolved: resolved))
        parts.append("voted \(voted) of \(eligible)")
        switch movement {
        case .up(let places): parts.append("up \(places)")
        case .down(let places): parts.append("down \(places)")
        case .same, .new: break
        }
        return parts.joined(separator: ", ") + "."
    }

    public static func leadTitle(leader: String, isCabal: Bool) -> String {
        "\(isCabal ? leader : firstName(leader)) took the lead"
    }

    public static func leadSubline(passed: String, isCabal: Bool, call: (kind: String, symbol: String)?, ratePct: Int)
        -> String
    {
        let who = isCabal ? passed : firstName(passed)
        guard let call else { return "Past \(who) · \(ratePct)%" }
        return "Past \(who), right on \(call.kind) \(call.symbol) · \(ratePct)%"
    }

    public static func leadGlyph(places: Int) -> String? { places > 0 ? "▲\(places)" : nil }

    public static func leadSpoken(
        leader: String, passed: String, isCabal: Bool, call: (kind: String, symbol: String)?, ratePct: Int
    ) -> String {
        let reason = call.map { ", right on \($0.kind) \($0.symbol)" } ?? ""
        let (a, b) = isCabal ? (leader, passed) : (firstName(leader), firstName(passed))
        return "\(a) took the lead from \(b)\(reason), \(ratePct) percent."
    }

    public static let shareStanding = "Share your standing"

    public static func standing(rank: Int, total: Int, ratePct: Int?, period: CallPeriod, tab: CallBoardTab) -> String {
        let when = period == .all ? "all time" : periodWord(period)
        let rate = ratePct.map { " · \($0)% right calls" } ?? ""
        return "I'm #\(rank) of \(total) on Monaco \(when)\(rate)"
    }

    private static func firstName(_ name: String) -> String { String(name.split(separator: " ").first ?? "") }
}
