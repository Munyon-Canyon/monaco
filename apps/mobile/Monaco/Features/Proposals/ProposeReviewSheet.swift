import MonacoCore
import SwiftUI

struct ProposeReviewSheet: View {
    @State private var isSending = false
    private let service: MonacoCore.ProposeService
    private let cabalID: String
    private let draft: MonacoCore.ProposalDraft
    private let preview: MonacoCore.ProposePreview
    private let trade: MonacoCore.ProposeTrade
    private let buyName: String?
    private let bestChain: MonacoCore.AssetBestChain?
    private let logoURL: URL?
    private let memory: MonacoCore.ProposeReviewMemory
    private let sendsOnAppear: Bool
    var onChangeCabal: (() -> Void)?

    init(
        service: MonacoCore.ProposeService, cabalID: String, draft: MonacoCore.ProposalDraft,
        preview: MonacoCore.ProposePreview, trade: MonacoCore.ProposeTrade, buyName: String? = nil,
        bestChain: MonacoCore.AssetBestChain? = nil, logoURL: URL? = nil, memory: MonacoCore.ProposeReviewMemory,
        sendsOnAppear: Bool = false, onChangeCabal: (() -> Void)? = nil
    ) {
        self.service = service
        self.cabalID = cabalID
        self.draft = draft
        self.preview = preview
        self.trade = trade
        self.buyName = buyName
        self.bestChain = bestChain
        self.logoURL = logoURL
        self.memory = memory
        self.sendsOnAppear = sendsOnAppear
        self.onChangeCabal = onChangeCabal
    }

    var body: some View {
        ProposeReviewScreen(
            service: service, cabalID: cabalID, draft: draft, preview: preview, trade: trade, buyName: buyName,
            bestChain: bestChain, logoURL: logoURL, memory: memory, isSending: $isSending, sendsOnAppear: sendsOnAppear,
            onChangeCabal: onChangeCabal
        )
        .monacoSheet(
            title: trade.isSell ? "Propose sell" : "Propose buy", titleIdentifier: "propose-review-sheet",
            detents: .tall,
            showsClose: true
        )
        .interactiveDismissDisabled(isSending)
    }
}
