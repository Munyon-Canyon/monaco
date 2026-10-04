import MonacoAPI

public enum FeedAccent: Equatable, Sendable {
    case ink
    case positive
    case negative
    case muted

    public init(kind: String, tone: String) {
        switch (FeedKind(rawValue: kind), tone) {
        case (_, "positive"): self = .positive
        case (_, "negative"): self = .negative
        case (.proposal, _), (.trade, _): self = .ink
        default: self = .muted
        }
    }

    public init(_ item: Components.Schemas.FeedItem) {
        self.init(kind: item.kind, tone: item.tone)
    }
}
