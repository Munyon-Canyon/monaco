import MonacoAPI
import MonacoCore
import SwiftUI

enum FeedEvent: Equatable {
    enum Block: Equatable {
        case asset(FeedSide)
        case price
        case quote
        case cabal
    }

    case proposal(FeedSide)
    case trade(FeedSide)
    case outcome(FeedSide)
    case priceMove
    case cabal
    case discussion
    case update

    static let discussionKind = "reply"
    static let outcomeKind = "threshold"

    init(_ item: Components.Schemas.FeedItem) {
        let side = FeedFigureFormat.side(item)
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

    init(_ item: Components.Schemas.FeedItem, event: FeedEvent) {
        ticker = AssetSymbolFormatter.display(item.symbol ?? "")
        company = item.assetName
        amount = FeedFigureFormat.amount(item)
        thesis = item.body.flatMap { $0.isEmpty ? nil : $0 }
        if case .trade = event {
            detail = FeedFigureFormat.detail(item) ?? item.detail.flatMap { $0.isEmpty ? nil : $0 }
            status = "Executed"
        } else {
            detail = FeedFigureFormat.detail(item)
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

    init(_ item: Components.Schemas.FeedItem) {
        ticker = AssetSymbolFormatter.display(item.symbol ?? "")
        company = item.assetName
        change = FeedFigureFormat.change(item)
    }

    var spoken: String {
        [ticker, company, change.map { "\($0) in 24 hours" }].compactMap { $0 }.joined(separator: ", ")
    }
}

struct FeedReturnFacts: Equatable {
    let text: String
    let spoken: String
    let isPrivate: Bool
    let color: Color

    init?(_ authorReturn: FeedAuthorReturn?) {
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
