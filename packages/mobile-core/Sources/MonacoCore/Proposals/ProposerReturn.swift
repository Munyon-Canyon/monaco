import Foundation

public struct ProposerReturn: Equatable, Sendable {
    public enum Tone: Equatable, Sendable {
        case gain
        case loss
        case muted
    }

    public let text: String
    public let tone: Tone

    public static let privateText = "P&L private"

    public init?(bps: Int64?, isPrivate: Bool) {
        switch FeedFigureFormat.authorReturn(bps: bps, isPrivate: isPrivate) {
        case .percent(let percent):
            text = "\(percent) · 1M"
            tone = Self.tone(percent)
        case .hidden:
            text = Self.privateText
            tone = .muted
        case nil:
            return nil
        }
    }

    private static func tone(_ percent: String) -> Tone {
        if !percent.contains(where: { $0.isNumber && $0 != "0" }) { return .muted }
        return percent.hasPrefix("+") ? .gain : .loss
    }
}
