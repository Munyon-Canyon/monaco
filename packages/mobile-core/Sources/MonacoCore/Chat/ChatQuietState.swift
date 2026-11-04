import Foundation

public enum ChatQuietState {
    public enum Kind: Equatable, Sendable {
        case none
        case fresh
        case quiet
    }

    public enum ActionID: Equatable, Sendable {
        case propose
        case poll
        case shareStock
    }

    public struct Action: Equatable, Sendable {
        public let id: ActionID
        public let title: String
        public let line: String
    }

    public static let title = GroupChatCopy.quietTitle
    public static let quietAfter: TimeInterval = 86_400

    public static func kind(rows: [ChatRow], openProposals: Int, now: Date) -> Kind {
        kind(messages: rows.map(\.message), openProposals: openProposals, now: now)
    }

    public static func kind(messages: [ChatMessage], openProposals: Int, now: Date) -> Kind {
        if openProposals > 0 { return .none }
        let newest =
            messages
            .filter { $0.kind == .user || $0.kind == .proposal }
            .map(\.createdAt)
            .max()
        guard let newest else { return .fresh }
        return now.timeIntervalSince(newest) >= quietAfter ? .quiet : .none
    }

    public static func line(memberCount: Int, pot: String?, figures: CabalPositionRowFigures?) -> String {
        var parts = [memberCount == 1 ? "1 member" : "\(memberCount) members"]
        guard let pot else { return parts[0] }
        parts.append("\(UsdAmountFormatter.format(decimalString: pot)) pot")
        if let figures {
            var yours = "yours \(UsdAmountFormatter.format(decimalString: figures.equityUsd))"
            switch figures.change {
            case .percent(let ratio): yours += " \(PercentReturnFormatter.format(ratio))"
            case .dollars(let amount): yours += " \(SignedUsdFormatter.format(amount))"
            case .unavailable: break
            }
            parts.append(yours)
        }
        return parts.joined(separator: " \u{00B7} ")
    }

    public static func actions(canPropose: Bool, voterCount: Int) -> [Action] {
        var actions: [Action] = []
        if canPropose {
            actions.append(
                Action(id: .propose, title: GroupChatCopy.quietPropose, line: proposeLine(voterCount: voterCount)))
        }
        actions.append(Action(id: .poll, title: GroupChatCopy.quietPoll, line: GroupChatCopy.quietPollLine))
        actions.append(
            Action(id: .shareStock, title: GroupChatCopy.quietShareStock, line: GroupChatCopy.quietShareStockLine))
        return actions
    }

    public static func composerPlaceholder(kind: Kind, cabalName: String?) -> String {
        switch kind {
        case .none: GroupChatCopy.composerPlaceholder(cabalName: nil)
        case .fresh, .quiet: GroupChatCopy.composerPlaceholder(cabalName: cabalName)
        }
    }

    private static let spelled = [
        2: "two", 3: "three", 4: "four", 5: "five", 6: "six", 7: "seven", 8: "eight", 9: "nine", 10: "ten",
    ]

    private static func proposeLine(voterCount: Int) -> String {
        let base = "Pick a stock and an amount, then"
        if voterCount <= 1 { return "\(base) you vote." }
        guard let word = spelled[voterCount] else { return "\(base) everyone votes." }
        return "\(base) the \(word) of you vote."
    }
}
