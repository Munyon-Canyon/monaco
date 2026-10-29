import Foundation
import MonacoAPI

public enum LedgerCopy {
    public static let notReady = "This week's Ledger isn't ready yet."
    public static let dash = "—"
    public static let fullLedgerTitle = "Full Ledger"
    public static let fullLedgerSubtitle = "Every call, the miss and the top reply"
    public static let noBestCallTitle = "No call landed this week"
    public static let noBestCallSubtitle = "Every call is in the Full Ledger"
    public static let shareTitle = "Share the Ledger"
    public static let loadingLabel = "Loading the Ledger"
    public static let everyCall = "Every call"
    public static let theMiss = "The miss"
    public static let topReply = "Top reply"
    public static let noCalls = "No call resolved this week."

    public static let votedLabel = "Voted"
    public static let rightLabel = "Right calls"
    public static let sharpestLabel = "Sharpest"

    public enum Tone: Equatable, Sendable {
        case gain, loss, flat
    }

    private static let months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

    public static func isNotReady(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error else { return false }
        return problem.code == .known(.ledgerNotFound)
    }

    public static func title(start: String, end: String) -> String {
        guard let from = parts(start), let to = parts(end) else { return "The Ledger" }
        let first = "\(months[from.month - 1]) \(from.day)"
        let last = from.month == to.month ? "\(to.day)" : "\(months[to.month - 1]) \(to.day)"
        return "The Ledger · \(first) – \(last)"
    }

    private static func parts(_ date: String) -> (month: Int, day: Int)? {
        let pieces = date.split(separator: "-")
        guard pieces.count == 3, let month = Int(pieces[1]), let day = Int(pieces[2]), (1...12).contains(month) else {
            return nil
        }
        return (month, day)
    }

    static func tenths(_ bps: Int) -> Int { (abs(bps) + 5) / 10 }

    private static func decimal(_ tenths: Int) -> String { "\(tenths / 10).\(tenths % 10)" }

    public static func signedPercent(bps: Int) -> String {
        let rounded = tenths(bps)
        guard rounded > 0 else { return "0.0%" }
        return "\(bps < 0 ? "−" : "+")\(decimal(rounded))%"
    }

    public static func tone(bps: Int) -> Tone {
        let rounded = tenths(bps)
        if rounded == 0 { return .flat }
        return bps < 0 ? .loss : .gain
    }

    public static func hero(bps: Int64?) -> String {
        bps.map { signedPercent(bps: Int($0)) } ?? dash
    }

    public static func heroTone(bps: Int64?) -> Tone {
        bps.map { tone(bps: Int($0)) } ?? .flat
    }

    public static func heroSubtitle(cabals: Int) -> String {
        "your stake that week · \(cabals) \(cabals == 1 ? "cabal" : "cabals")"
    }

    public static func heroSpoken(bps: Int64?, cabals: Int) -> String {
        let across = "across \(cabals) \(cabals == 1 ? "cabal" : "cabals")"
        guard let bps else { return "No return for your stake that week, \(across)." }
        let rounded = tenths(Int(bps))
        switch tone(bps: Int(bps)) {
        case .gain: return "Up \(decimal(rounded)) percent on your stake that week, \(across)."
        case .loss: return "Down \(decimal(rounded)) percent on your stake that week, \(across)."
        case .flat: return "Flat on your stake that week, \(across)."
        }
    }

    public static func ofTotal(_ count: Int, _ total: Int) -> String { "\(count) of \(total)" }

    public static func sharpest(_ name: String?) -> String {
        name?.split(separator: " ").first.map(String.init) ?? dash
    }

    public static func callTitle(kind: LedgerCall.Kind, symbol: String) -> String {
        "\(kind == .buy ? "Buy" : "Sell") \(symbol)"
    }

    public static func bestCallTitle(_ call: LedgerCall) -> String {
        "Best call · \(callTitle(kind: call.kind, symbol: call.symbol))"
    }

    public static func bestCallSubtitle(_ call: LedgerCall) -> String {
        "\(call.cabalName) · you voted \(call.votedYes ? "Yes" : "No")"
    }

    public static func callRowTitle(_ call: LedgerCall) -> String {
        "\(callTitle(kind: call.kind, symbol: call.symbol)) · \(call.cabalName)"
    }

    public static func callRowSubtitle(_ call: LedgerCall) -> String {
        "you voted \(call.votedYes ? "Yes" : "No") · \(call.right ? "right" : "missed")"
    }

    public static func replyCaption(_ reply: LedgerReply) -> String {
        "\(reply.topEmoji) \(reply.topEmojiCount) · \(reply.proposalLabel)"
    }
}
