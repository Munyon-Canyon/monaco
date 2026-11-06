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
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @MonacoReduceTransparency private var reduceTransparency
    @FocusState private var thesisFocused: Bool
    @State private var model: MonacoCore.ProposeReviewModel?
    @State private var cabalFailed = false
    @State private var showsMarketOrder = false
    @Binding private var isSending: Bool
    @Binding private var isWriting: Bool
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
    private let focusesThesis: Bool
    private let onChangeCabal: (() -> Void)?

    init(
        service: MonacoCore.ProposeService, cabalID: String, draft: MonacoCore.ProposalDraft,
        preview: MonacoCore.ProposePreview, trade: MonacoCore.ProposeTrade, buyName: String? = nil,
        bestChain: MonacoCore.AssetBestChain? = nil, logoURL: URL? = nil, memory: MonacoCore.ProposeReviewMemory,
        isSending: Binding<Bool> = .constant(false), isWriting: Binding<Bool> = .constant(false),
        sendsOnAppear: Bool = false, focusesThesis: Bool = false, onChangeCabal: (() -> Void)? = nil
    ) {
        self.focusesThesis = focusesThesis
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
        _isWriting = isWriting
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
        .onChange(of: thesisFocused) { _, focused in isWriting = focused }
        .onChange(of: isWriting) { _, writing in if !writing { thesisFocused = false } }
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
        let writing = thesisFocused
        return ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                if writing { foldedTrade(model) }
                thesisBox(model)
                if !writing {
                    tradeCard(model)
                    if model.isPreIpoBuy {
                        ProposalPreIpoLedger(
                            lines: model.preIpoLedger, eligibility: .eligible, showsEligibility: false,
                            identifier: "propose-review-preipo-ledger")
                    } else {
                        factsLine(model)
                    }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.s)
            .animation(Self.swap(reduceMotion: reduceMotion), value: writing)
        }
        .scrollBounceBehavior(.basedOnSize)
        .scrollDismissesKeyboard(.interactively)
        .safeAreaInset(edge: .bottom) { footer(model, writing: writing) }
        .sheet(isPresented: $showsMarketOrder) {
            MonacoInfoSheet(
                title: ProposeFlowCopy.marketOrderTitle, message: ProposeFlowCopy.marketOrderBody, rows: [],
                identifier: "propose-review-market-order")
        }
    }

    private func assetRow(_ model: MonacoCore.ProposeReviewModel) -> some View {
        let stacked = dynamicTypeSize.isAccessibilitySize
        let layout =
            stacked
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        return layout {
            assetTitle(model)
            if !dynamicTypeSize.isAccessibilitySize { Spacer(minLength: 0) }
            VStack(alignment: stacked ? .leading : .trailing, spacing: MonacoTheme.Space.xs) {
                Text(model.amountLabel).moneyFigure(.large).foregroundStyle(MonacoTheme.ink).minimumScaleFactor(0.6)
                    .lineLimit(1)
                if let treasury = model.treasuryLine {
                    Text(treasury).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                        .accessibilityIdentifier("propose-review-treasury")
                }
            }
        }
        .padding(.vertical, MonacoTheme.Space.sm)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("propose-review-header")
    }

    private func cabalRow(_ model: MonacoCore.ProposeReviewModel) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            HStack(spacing: MonacoTheme.Space.sm) {
                CabalMark(groupId: cabalID, name: model.cabal.name, size: 32)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(model.cabal.name).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.ink)
                        .fixedSize(horizontal: false, vertical: true)
                    Text(model.cabalLine).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            .accessibilityElement(children: .combine)
            Spacer(minLength: 0)
            if let onChangeCabal {
                Button(ProposeFlowCopy.changeCabal, action: onChangeCabal)
                    .buttonStyle(.monacoText)
                    .disabled(model.isSending)
                    .accessibilityIdentifier("propose-review-change-cabal")
            }
        }
        .padding(.vertical, MonacoTheme.Space.sm)
    }

    private func footer(_ model: MonacoCore.ProposeReviewModel, writing: Bool) -> some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            if model.errorMessage != nil { errorNotice(model) }
            HStack(spacing: 0) {
                if writing { Spacer(minLength: 0) }
                Button {
                    Task { await model.send() }
                } label: {
                    Label(model.sendTitle, systemImage: "paperplane.fill")
                }
                .buttonStyle(.monacoPrimary)
                .monacoFullWidthButtons(!writing)
                .fixedSize(horizontal: writing, vertical: false)
                .disabled(!model.canSend)
                .accessibilityIdentifier("propose-review-send")
            }
            if let problem = model.chainProblem {
                Text(problem).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("propose-review-chain-problem")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .padding(.bottom, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity)
        .background(MonacoTheme.sheetSurface)
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

extension ProposeReviewScreen {
    static func swap(reduceMotion: Bool) -> Animation {
        reduceMotion ? .easeInOut(duration: 0.2) : .spring(response: 0.3, dampingFraction: 0.85)
    }

    private func tradeCard(_ model: MonacoCore.ProposeReviewModel) -> some View {
        VStack(spacing: 0) {
            assetRow(model)
            MonacoRule()
            cabalRow(model)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .background(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                .fill(MonacoTheme.surfaceSunken)
        )
        .environment(\.monacoKeyRowsInset, true)
        .transition(.opacity)
    }

    private func foldedTrade(_ model: MonacoCore.ProposeReviewModel) -> some View {
        let stacked = dynamicTypeSize.isAccessibilitySize
        let layout =
            stacked
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
        return layout {
            StockMark(
                symbol: model.symbol, displayName: model.assetDisplayName, assetKind: model.assetKind,
                size: Self.foldedMarkSize, logoURL: logoURL)
            (Text(model.foldedTradeLine).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.ink)
                + Text(" · \(model.cabal.name)").font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText))
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("propose-review-trade-folded")
        .transition(.opacity)
    }

    static let foldedMarkSize: CGFloat = 24
    static let thesisMinHeight: CGFloat = 96

    private func thesisBox(_ model: MonacoCore.ProposeReviewModel) -> some View {
        let stacked = dynamicTypeSize.isAccessibilitySize
        let face = model.cabal.faces[environment.viewer?.userID ?? ""]
        let profile = environment.sessionStore.profile
        let avatar = MonacoAvatar(
            photoURL: face?.photoURL ?? profile?.photoURL?.absoluteString,
            displayName: face?.name ?? profile?.displayName ?? "", size: Self.avatarSize,
            seed: environment.viewer?.userID
        )
        .accessibilityHidden(true)
        let field = TextField(
            "", text: Binding(get: { model.thesis }, set: { model.setThesis($0) }),
            prompt: Text(model.thesisPrompt).foregroundStyle(MonacoTheme.secondaryText), axis: .vertical
        )
        .lineLimit(1...8)
        .font(MonacoTheme.Typo.body)
        .foregroundStyle(MonacoTheme.ink)
        .tint(MonacoTheme.accent)
        .focused($thesisFocused)
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .accessibilityLabel(model.thesisPrompt)
        .accessibilityHint(Self.thesisHint)
        .accessibilityIdentifier("propose-review-thesis-field")
        return Group {
            if stacked {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    HStack {
                        avatar
                        Spacer(minLength: 0)
                        optionalLabel(model)
                    }
                    field
                }
            } else {
                HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                    avatar
                    field
                    optionalLabel(model)
                }
            }
        }
        .padding(MonacoTheme.Space.m)
        .padding(.bottom, model.thesisCounter == nil ? 0 : MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, minHeight: Self.thesisMinHeight, alignment: .topLeading)
        .background(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                .fill(MonacoTheme.surfaceSunken)
        )
        .overlay(alignment: .bottomTrailing) {
            counter(model).padding(MonacoTheme.Space.m)
        }
        .contentShape(Rectangle())
        .onTapGesture { thesisFocused = true }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("propose-review-thesis")
        .task {
            guard focusesThesis else { return }
            try? await Task.sleep(for: .milliseconds(400))
            thesisFocused = true
        }
    }

    private func optionalLabel(_ model: MonacoCore.ProposeReviewModel) -> some View {
        Text(model.thesisOptionalLabel).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
            .accessibilityHidden(true)
    }

    @ViewBuilder
    private func counter(_ model: MonacoCore.ProposeReviewModel) -> some View {
        if let text = model.thesisCounter {
            Text(text).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
                .monospacedDigit()
                .accessibilityLabel(Self.counterSpeech(text))
                .accessibilityIdentifier("propose-review-thesis-counter")
        }
    }

    static let avatarSize: CGFloat = 32

    static var thesisHint: String { "Optional. Up to \(MonacoCore.ProposeReasonRules.thesisLimit) characters." }

    static func counterSpeech(_ counter: String) -> String {
        counter.replacingOccurrences(of: " / ", with: " of ") + " characters"
    }

    @ViewBuilder
    func factsLine(_ model: MonacoCore.ProposeReviewModel) -> some View {
        let leading = model.factsLeading
        let trailing = model.factsTrailing
        if !leading.isEmpty || !trailing.isEmpty {
            ViewThatFits(in: .horizontal) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    Text(leading)
                    Spacer(minLength: 0)
                    Text(trailing)
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(leading)
                    Text(trailing)
                }
            }
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("propose-review-facts")
        }
    }

    private func assetTitle(_ model: MonacoCore.ProposeReviewModel) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        return layout {
            StockMark(
                symbol: model.symbol, displayName: model.assetDisplayName, assetKind: model.assetKind,
                size: ProposeAmountScreen.markSize, logoURL: logoURL)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(model.assetDisplayName).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)
                HStack(spacing: MonacoTheme.Space.xs) {
                    Text(model.assetLine).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize(horizontal: false, vertical: true)
                    if !model.isSell {
                        Button {
                            showsMarketOrder = true
                        } label: {
                            Image(systemName: MonacoIcon.info.symbol).foregroundStyle(MonacoTheme.secondaryText)
                        }
                        .frame(minWidth: CommentRowMetrics.replyTarget, minHeight: CommentRowMetrics.replyTarget)
                        .contentShape(Rectangle())
                        .padding(.vertical, -(CommentRowMetrics.replyTarget - MonacoTheme.Space.m) / 2)
                        .accessibilityLabel(ProposeFlowCopy.marketOrderTitle)
                        .accessibilityIdentifier("propose-review-market-order-info")
                    }
                }
            }
        }
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
