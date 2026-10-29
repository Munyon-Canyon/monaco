import MonacoAPI
import MonacoCore
import SwiftUI

private struct SolidSheetBackground: ViewModifier {
    let isSolid: Bool

    @ViewBuilder
    func body(content: Content) -> some View {
        if isSolid {
            content.presentationBackground(MonacoTheme.canvas)
        } else {
            content
        }
    }
}

struct ProposeReviewScreen: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(\.dismiss) private var dismiss
    @MonacoReduceTransparency private var reduceTransparency
    @State private var model: MonacoCore.ProposeReviewModel?
    @State private var cabalFailed = false
    @State private var showsMarketOrder = false
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
    private let onChangeCabal: (() -> Void)?

    init(
        service: MonacoCore.ProposeService, cabalID: String, draft: MonacoCore.ProposalDraft,
        preview: MonacoCore.ProposePreview, trade: MonacoCore.ProposeTrade, buyName: String? = nil,
        bestChain: MonacoCore.AssetBestChain? = nil, logoURL: URL? = nil, memory: MonacoCore.ProposeReviewMemory,
        isSending: Binding<Bool> = .constant(false), sendsOnAppear: Bool = false,
        onChangeCabal: (() -> Void)? = nil
    ) {
        self.onChangeCabal = onChangeCabal
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
        .modifier(SolidSheetBackground(isSolid: reduceTransparency))
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
                MonacoCard(style: .inset) {
                    VStack(spacing: 0) {
                        assetRow(model)
                        MonacoRule()
                        cabalRow(model)
                        if model.showsThesis {
                            MonacoRule()
                            thesisRow(model)
                        }
                    }
                }
                if model.isPreIpoBuy {
                    ProposalPreIpoLedger(
                        lines: model.preIpoLedger, eligibility: .eligible, showsEligibility: false,
                        identifier: "propose-review-preipo-ledger")
                } else {
                    StatGrid(
                        items: model.stats.map { StatItem(label: $0.label, value: $0.value) }
                    )
                    .accessibilityElement(children: .contain)
                    .accessibilityIdentifier("propose-review-stats")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.s)
        }
        .scrollBounceBehavior(.basedOnSize)
        .safeAreaInset(edge: .bottom) { footer(model) }
        .sheet(isPresented: $showsMarketOrder) {
            MonacoInfoSheet(
                title: ProposeFlowCopy.marketOrderTitle, message: ProposeFlowCopy.marketOrderBody, rows: [],
                identifier: "propose-review-market-order")
        }
    }

    private func assetRow(_ model: MonacoCore.ProposeReviewModel) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            StockMark(
                symbol: model.symbol, displayName: model.assetDisplayName, assetKind: model.assetKind,
                size: ProposeAmountScreen.markSize, logoURL: logoURL)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(model.assetDisplayName).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.ink)
                HStack(spacing: MonacoTheme.Space.xs) {
                    Text(model.assetLine).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                    if !model.isSell {
                        Button {
                            showsMarketOrder = true
                        } label: {
                            Image(systemName: MonacoIcon.info.symbol).foregroundStyle(MonacoTheme.secondaryText)
                        }
                        .frame(minWidth: CommentRowMetrics.replyTarget, minHeight: CommentRowMetrics.replyTarget)
                        .contentShape(Rectangle())
                        .accessibilityLabel(ProposeFlowCopy.marketOrderTitle)
                        .accessibilityIdentifier("propose-review-market-order-info")
                    }
                }
            }
            Spacer(minLength: 0)
            Text(model.amountLabel).moneyFigure(.large).foregroundStyle(MonacoTheme.ink).minimumScaleFactor(0.6)
                .lineLimit(1)
        }
        .padding(.vertical, MonacoTheme.Space.sm)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("propose-review-header")
    }

    private func cabalRow(_ model: MonacoCore.ProposeReviewModel) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            CabalMark(groupId: cabalID, name: model.cabal.name, size: 32)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(model.cabal.name).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.ink)
                Text(model.cabalLine).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
            }
            Spacer(minLength: 0)
            if let onChangeCabal {
                Button(ProposeFlowCopy.changeCabal, action: onChangeCabal)
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier("propose-review-change-cabal")
            }
        }
        .padding(.vertical, MonacoTheme.Space.sm)
    }

    private func thesisRow(_ model: MonacoCore.ProposeReviewModel) -> some View {
        Text(model.reason ?? ProposeFlowCopy.whyThisTrade)
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(model.reason == nil ? MonacoTheme.secondaryText : MonacoTheme.bodyText)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.vertical, MonacoTheme.Space.sm)
            .accessibilityIdentifier("propose-review-thesis")
    }

    private func footer(_ model: MonacoCore.ProposeReviewModel) -> some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            if model.errorMessage != nil { errorNotice(model) }
            Button {
                Task { await model.send() }
            } label: {
                Label(model.sendTitle, systemImage: "paperplane.fill")
            }
            .buttonStyle(.monacoPrimary)
            .disabled(!model.canSend)
            .accessibilityIdentifier("propose-review-send")
            if let problem = model.chainProblem {
                Text(problem).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("propose-review-chain-problem")
            }
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

    private func preIpoProfile() async -> PreIpoDetail.Profile? {
        let symbol = trade.symbol
        let detail = try? await environment.api.read { client in
            try await client.getAsset(path: .init(symbol: symbol)).ok.body.json
        }
        return detail.flatMap { PreIpoDetail($0)?.profile }
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
            let built = memory.hold(
                MonacoCore.ProposeReviewModel(
                    service: service, cabalID: cabalID, cabal: cabal, draft: draft, preview: preview, trade: trade,
                    buyName: buyName, bestChain: bestChain))
            if built.isPreIpoBuy { built.setPreIpo(await preIpoProfile()) }
            model = built
        } catch {
            cabalFailed = true
            return
        }
        if sendsOnAppear { await model?.send() }
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
