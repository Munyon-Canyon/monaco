import MonacoCore
import SwiftUI

struct ProposeReviewScreen: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(\.dismiss) private var dismiss
    @MonacoReduceTransparency private var reduceTransparency
    @State private var model: MonacoCore.ProposeReviewModel?
    @State private var cabalFailed = false
    @Binding private var isSending: Bool
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

    init(
        service: MonacoCore.ProposeService, cabalID: String, draft: MonacoCore.ProposalDraft,
        preview: MonacoCore.ProposePreview, trade: MonacoCore.ProposeTrade, buyName: String? = nil,
        bestChain: MonacoCore.AssetBestChain? = nil, logoURL: URL? = nil, memory: MonacoCore.ProposeReviewMemory,
        isSending: Binding<Bool> = .constant(false), sendsOnAppear: Bool = false
    ) {
        self.buyName = buyName
        self.bestChain = bestChain
        self.logoURL = logoURL
        self.memory = memory
        self.sendsOnAppear = sendsOnAppear
        self.service = service
        self.cabalID = cabalID
        self.draft = draft
        self.preview = preview
        self.trade = trade
        _isSending = isSending
    }

    var body: some View {
        Group {
            if let model {
                content(model)
            } else if cabalFailed {
                EmptyState.loadFailed(identifier: "propose-review-error") { Task { await load() } }
                    .frame(maxHeight: .infinity, alignment: .top)
            } else {
                ProposeReviewSkeleton()
            }
        }
        .task { await load() }
        .onChange(of: model?.isSending ?? false) { _, sending in isSending = sending }
        .onDisappear { if model?.proposalID != nil { closeFlow() } }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("propose-review-screen")
    }

    private var tab: MainTab { hostMainTab ?? environment.navigator.selectedTab }

    private func closeFlow() { environment.navigator.closeProposeFlow(in: tab) }

    @ViewBuilder
    private func content(_ model: MonacoCore.ProposeReviewModel) -> some View {
        if let proposalID = model.proposalID {
            sent(model, proposalID: proposalID)
        } else {
            form(model)
        }
    }

    private func sent(_ model: MonacoCore.ProposeReviewModel, proposalID: String) -> some View {
        OutcomeView(tone: .done, title: ProposeFlowCopy.proposalSentTitle, message: model.sentMessage) {
            Button(ProposeFlowCopy.viewProposal) {
                closeFlow()
                environment.navigator.open(ProposalRoute(proposalID: proposalID), in: tab)
                dismiss()
            }
            .buttonStyle(.monacoPrimary)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("propose-review-view-proposal")
        }
        .frame(maxHeight: .infinity)
        .accessibilityIdentifier("propose-review-sent")
    }

    private func form(_ model: MonacoCore.ProposeReviewModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                ProposeReviewHero(model: model, markSize: ProposeAmountScreen.markSize, logoURL: logoURL)
                Text(model.cabal.name)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.brand)
                    .padding(.vertical, MonacoTheme.Space.s)
                    .padding(.horizontal, MonacoTheme.Space.sm)
                    .background(MonacoTheme.brandSoft, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip))
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                Text(model.voteCaption)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .accessibilityIdentifier("propose-review-vote-caption")
                MonacoKeyRows {
                    ForEach(Array(model.rows.enumerated()), id: \.offset) { index, row in
                        ReceiptLine(
                            label: row.label, value: Self.value(row), isLast: index == model.rows.count - 1)
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                if let bestPrice = model.bestPrice {
                    BestPriceSection(
                        row: BestPriceRow(
                            bestPrice, caption: model.isEstimate ? MonacoCore.ProposeReviewModel.estimateCaption : nil))
                }
                ProposeChainSection(model: model)
                if let reason = model.reason { thesis(model.reasonTitle, reason) }
            }
            .padding(.vertical, MonacoTheme.Space.s)
        }
        .scrollBounceBehavior(.basedOnSize)
        .safeAreaInset(edge: .bottom) { footer(model) }
    }

    private func thesis(_ title: String, _ text: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            Text(title).font(MonacoTheme.Typo.subheadStrong).foregroundStyle(MonacoTheme.ink)
            Text(text)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.bodyText)
                .lineSpacing(MonacoTheme.Space.xs)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("propose-review-thesis")
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private func footer(_ model: MonacoCore.ProposeReviewModel) -> some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            if model.errorMessage != nil { errorNotice(model) }
            Button(model.sendTitle) { Task { await model.send() } }
                .buttonStyle(.monacoPrimary)
                .disabled(!model.canSend)
                .accessibilityIdentifier("propose-review-send")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .padding(.bottom, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity)
        .background(reduceTransparency ? AnyShapeStyle(MonacoTheme.surface) : AnyShapeStyle(.regularMaterial))
        .overlay(alignment: .top) { MonacoRule() }
    }

    @ViewBuilder
    private func errorNotice(_ model: MonacoCore.ProposeReviewModel) -> some View {
        let detail = model.errorMessage.flatMap { $0 == model.errorNotice ? nil : $0 }
        MonacoNotice(
            .error, title: detail == nil ? nil : model.errorNotice, message: detail ?? model.errorNotice
        )
        .accessibilityIdentifier("propose-review-error-notice")
    }

    private static func value(_ row: MonacoCore.ProposeReviewModel.Row) -> ReceiptLine.Value {
        ["Price", "Cabal gets", "Cabal keeps", "Voting window", "Votes to pass"].contains(row.label)
            ? .data(row.value) : .words(row.value)
    }

    private func load() async {
        guard model == nil else { return }
        if let held = memory.model(for: draft) {
            model = held
            return
        }
        cabalFailed = false
        do {
            let cabal = try await MonacoCore.ProposeCabalInfo.load(api: environment.api, cabalID: cabalID)
            model = memory.hold(
                MonacoCore.ProposeReviewModel(
                    service: service, cabalID: cabalID, cabal: cabal, draft: draft, preview: preview, trade: trade,
                    buyName: buyName, bestChain: bestChain))
        } catch {
            cabalFailed = true
            return
        }
        if sendsOnAppear { await model?.send() }
    }
}

private struct ProposeReviewHero: View {
    let model: MonacoCore.ProposeReviewModel
    let markSize: CGFloat
    let logoURL: URL?

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            StockMark(
                symbol: model.symbol, displayName: model.assetDisplayName, assetKind: model.assetKind, size: markSize,
                logoURL: logoURL)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(model.operationLine).font(MonacoTheme.Typo.body).foregroundStyle(MonacoTheme.ink)
                Text(model.heroAmount)
                    .moneyFigure(.large)
                    .foregroundStyle(MonacoTheme.ink)
                    .minimumScaleFactor(0.6)
                    .lineLimit(1)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(model.heroAccessibilityLabel)
        .accessibilityAddTraits(.isHeader)
        .accessibilityIdentifier("propose-review-header")
    }
}

private struct ProposeReviewSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            HStack(spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(
                    width: ProposeAmountScreen.markSize, height: ProposeAmountScreen.markSize,
                    radius: MonacoTheme.Radius.field)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    SkeletonBar(width: nil)
                    SkeletonBar(width: nil)
                }
            }
            SkeletonBar(width: nil)
            ForEach(0..<3, id: \.self) { _ in SkeletonBar(width: nil) }
            ForEach(0..<2, id: \.self) { _ in SkeletonBar(width: nil) }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .accessibilityHidden(true)
    }
}
