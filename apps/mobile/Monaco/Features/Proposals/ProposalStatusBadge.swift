import MonacoCore
import SwiftUI

extension StatusBadge {
    init(proposal badge: ProposalBadge) {
        self.init(badge.label, tone: badge.tone.statusTone)
    }
}

extension ProposalBadge.Tone {
    fileprivate var statusTone: StatusBadge.Tone {
        switch self {
        case .brand: .positive
        case .warn: .warn
        case .negative: .negative
        case .neutral: .neutral
        }
    }
}

extension MonacoNotice {
    init(proposal notice: ProposalCardCopy.Notice) {
        self.init(notice.tone.noticeTone, message: notice.message)
    }
}

extension ProposalCardCopy.NoticeTone {
    fileprivate var noticeTone: MonacoNotice.Tone {
        switch self {
        case .info: .info
        case .warn: .warn
        case .error: .error
        }
    }
}

extension ProposalSummary {
    func title(asset: ProposalAsset?) -> String {
        let name =
            asset.map { AssetDisplayName.format(catalogName: $0.displayName, kind: $0.kind) }
            ?? AssetSymbolFormatter.display(symbol, kind: .stock)
        return "\(kind == "sell" ? "Sell" : "Buy") \(name)"
    }
}
