import MonacoAPI
import MonacoCore
import SwiftUI

enum FeedEvent: Equatable {
    enum Block: Equatable {
        case asset(FeedFigures.Side)
        case price
        case quote
        case cabal
    }

    case proposal(FeedFigures.Side)
    case trade(FeedFigures.Side)
    case outcome(FeedFigures.Side)
    case priceMove
    case cabal
    case discussion
    case update

    static let discussionKind = "reply"
    static let outcomeKind = "threshold"

    init(_ item: Components.Schemas.FeedItem, figures: FeedFigures?) {
        let side = figures?.side ?? .buy
        switch item.kind {
        case FeedKind.proposal.rawValue: self = .proposal(side)
        case FeedKind.trade.rawValue: self = .trade(side)
        case FeedKind.priceMove.rawValue: self = .priceMove
        case FeedKind.cabalCreated.rawValue, FeedKind.memberJoined.rawValue: self = .cabal
        case Self.outcomeKind: self = .outcome(side)
        case Self.discussionKind: self = .discussion
        default: self = .update
        }
    }

    var pillLabel: String {
        switch self {
        case .proposal(.buy): "Proposal"
        case .proposal(.sell): "Propose sell"
        case .trade(.buy): "Buy"
        case .trade(.sell): "Sell"
        case .outcome: "Approved"
        case .priceMove: "Price move"
        case .cabal: "Cabal"
        case .discussion: "Discussion"
        case .update: "Update"
        }
    }

    var pillTone: StatusBadge.Tone {
        switch self {
        case .proposal(.buy), .trade(.buy), .cabal: .positive
        case .proposal(.sell), .trade(.sell): .negative
        case .outcome, .priceMove: .warn
        case .discussion: .discussion
        case .update: .neutral
        }
    }

    var statusColor: Color {
        if case .outcome = self { return MonacoTheme.warning }
        return MonacoTheme.secondaryText
    }

    var amountPhrase: String {
        switch self {
        case .proposal: "Proposed amount"
        case .trade(.sell): "Sale proceeds"
        case .trade(.buy): "Amount bought"
        case .outcome: "Approved amount"
        case .priceMove, .cabal, .discussion, .update: "Amount"
        }
    }

    var block: Block {
        switch self {
        case .proposal(let side), .trade(let side), .outcome(let side): .asset(side)
        case .priceMove: .price
        case .cabal: .cabal
        case .discussion, .update: .quote
        }
    }
}

struct FeedAssetFacts: Equatable {
    let ticker: String
    let company: String?
    let amount: String?
    let detail: String?
    let status: String?
    let thesis: String?

    init(_ item: Components.Schemas.FeedItem, figures: FeedFigures?, event: FeedEvent) {
        ticker = AssetSymbolFormatter.display(item.symbol ?? "")
        company = figures?.company
        amount = figures?.amount
        thesis = item.body.flatMap { $0.isEmpty ? nil : $0 }
        if case .trade = event {
            detail = figures?.detail ?? item.detail.flatMap { $0.isEmpty ? nil : $0 }
            status = "Executed"
        } else {
            detail = figures?.detail
            status = item.detail.flatMap { $0.isEmpty ? nil : $0 }
        }
    }

    func spoken(_ event: FeedEvent) -> String {
        [ticker, company, amount.map { "\(event.amountPhrase) \($0)" }, detail, status]
            .compactMap { $0 }.joined(separator: ", ")
    }
}

struct FeedPriceFacts: Equatable {
    let ticker: String
    let company: String?
    let change: String?
    let volume: String?

    init(_ item: Components.Schemas.FeedItem, figures: FeedFigures?) {
        ticker = AssetSymbolFormatter.display(item.symbol ?? "")
        company = figures?.company
        change = figures?.change ?? Self.change(in: item)
        volume = figures?.volume
    }

    var spoken: String {
        [ticker, company, change.map { "\($0) in 24 hours" }, volume.map { "24 hour token volume \($0)" }]
            .compactMap { $0 }.joined(separator: ", ")
    }

    static func change(in item: Components.Schemas.FeedItem) -> String? {
        guard let range = item.title.range(of: #"\d+(\.\d+)?%"#, options: .regularExpression) else { return nil }
        let percent = String(item.title[range])
        switch item.tone {
        case "positive": return "+" + percent
        case "negative": return "\u{2212}" + percent
        default: return nil
        }
    }
}

struct FeedReturnFacts: Equatable {
    let text: String
    let spoken: String
    let isPrivate: Bool
    let color: Color

    init?(_ authorReturn: FeedFigures.AuthorReturn?) {
        switch authorReturn {
        case .percent(let percent):
            text = "\(percent) · 1M"
            spoken = "Personal return \(PnLSpeech.percent(percent)) for one month"
            isPrivate = false
            color = MonacoTheme.signed(percent)
        case .hidden:
            text = "P&L private"
            spoken = "Personal performance private"
            isPrivate = true
            color = MonacoTheme.primaryText
        case nil:
            return nil
        }
    }
}

struct FeedOpensSoonData {
    var figures: [String: FeedFigures] = [:]
    var commentReturns: [String: FeedFigures.AuthorReturn] = [:]
    var hasUnreadActivity = false

    static let none = FeedOpensSoonData()
}

extension EnvironmentValues {
    @Entry var feedOpensSoon = FeedOpensSoonData.none
}

enum FeedPostPhase: Equatable {
    case loading
    case ready(Components.Schemas.FeedItem)
    case unavailable
    case failed

    @MainActor
    init(_ model: FeedItemDetailModel) {
        if let item = model.item {
            self = .ready(item)
        } else if case .problem(let problem) = model.itemError, [403, 404].contains(problem.status) {
            self = .unavailable
        } else if model.itemError != nil {
            self = .failed
        } else {
            self = .loading
        }
    }
}
