import MonacoCore
import SwiftUI

struct ProposeAmountSeed {
    var amount = ""
    var thesis = ""
    var requestsReview = false
    var opensReview = false
    var sendsReview = false
}

struct ProposeAmountScreen: View {
    let stock: ProposeStock
    var seed = ProposeAmountSeed()
    private let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @State private var showsReview = false
    @State private var model: ProposeAmountModel
    @State private var reviewMemory = MonacoCore.ProposeReviewMemory()
    @State private var amountText = ""

    init(
        service: MonacoCore.ProposeService, cabalID: String, stock: ProposeStock, trade: ProposeTrade? = nil,
        bestChains: AssetBestChainStore? = nil
    ) {
        self.stock = stock
        self.cabalID = cabalID
        let trade = trade ?? .buy(symbol: stock.symbol, kind: stock.assetKind, tokenDecimals: stock.tokenDecimals)
        _model = State(
            initialValue: ProposeAmountModel(
                service: service, cabalID: cabalID, trade: trade, clock: ContinuousClock(), bestChains: bestChains))
    }

    static let markSize: CGFloat = 48

    private var displayName: String { AssetDisplayName.format(catalogName: stock.name, kind: stock.assetKind) }
    private var operation: String { model.trade.isSell ? "Sell" : "Buy" }

    var body: some View {
        Group {
            if !model.trade.isSell, model.cashFailed {
                EmptyState.loadFailed(identifier: "propose-amount-cash-error") { Task { await model.load() } }
                    .frame(maxHeight: .infinity, alignment: .top)
            } else {
                content
            }
        }
        .monacoCanvas()
        .task { await model.load() }
        .task { await model.prefetchBestChain() }
        .task { await applySeed() }
        .task {
            guard !model.trade.isSell,
                let asset = try? await ProposalsRepository(api: environment.api).asset(symbol: stock.symbol)
            else { return }
            model.resolveAsset(kind: asset.kind, decimals: asset.decimals)
        }
        .navigationTitle("\(operation) \(displayName)")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("propose-amount-screen")
    }

    private var content: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                header
                ProposeContextCard(cabalID: cabalID, detail: contextDetail)
                amountEntry
                notices
                thesisSection
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.m)
        }
        .scrollBounceBehavior(.basedOnSize)
        .scrollDismissesKeyboard(.interactively)
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button(model.reviewTitle) { review() }
                    .buttonStyle(.monacoPrimary)
                    .disabled(!model.reviewEnabled(assetName: stock.name))
                    .accessibilityIdentifier("propose-amount-review")
            }
        }
        .sheet(isPresented: $showsReview) {
            if let preview = model.preview {
                ProposeReviewSheet(
                    service: MonacoCore.LiveProposeService(api: environment.api), cabalID: cabalID,
                    draft: model.draft, preview: preview, trade: model.trade,
                    buyName: displayName, bestChain: model.previewBestChain, memory: reviewMemory,
                    sendsOnAppear: seed.sendsReview)
            }
        }
    }

    private func applySeed() async {
        guard !seed.amount.isEmpty else { return }
        amountText = seed.amount
        model.setThesis(seed.thesis)
        guard seed.requestsReview else { return }
        for _ in 0..<50 where !model.reviewEnabled(assetName: stock.name) {
            try? await Task.sleep(for: .milliseconds(100))
        }
        if seed.opensReview { showsReview = true } else { review() }
    }

    private func review() {
        if model.requestReview(assetName: stock.name) { showsReview = true }
    }

    private var header: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            StockMark(
                symbol: stock.symbol, displayName: stock.name, assetKind: stock.assetKind, size: Self.markSize)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(displayName).font(MonacoTheme.Typo.section).foregroundStyle(MonacoTheme.ink)
                Text(stock.ticker).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted)
            }
            Spacer(minLength: 0)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(operation) \(displayName)")
        .accessibilityAddTraits(.isHeader)
        .accessibilityIdentifier("propose-amount-operation")
    }

    private var contextDetail: ProposeContextDetail {
        if case .sell(let holding) = model.trade {
            return .units("\(holding.quantity(of: holding.tokenAmount)) held")
        }
        guard let micros = model.spendableCashMicros else { return .loading }
        return .cash(micros: micros)
    }

    private var amountEntry: some View {
        AmountEntry(
            amountText: $amountText, max: max, presets: presets,
            caption: model.trade.isSell ? ProposeFlowCopy.sellAmountEyebrow : ProposeFlowCopy.buyAmountEyebrow,
            helper: model.helperText, overLimitHelper: model.helperText ?? "", isLoading: model.isLoading
        ) {
            VStack(spacing: MonacoTheme.Space.s) {
                if let note = model.sellQuantityNote { AmountEntryNote(note) }
                if let note = model.buyQuantityNote {
                    AmountEntryNote(note).accessibilityIdentifier("propose-amount-quantity")
                }
            }
            .padding(.top, MonacoTheme.Space.s)
        }
        .padding(.top, MonacoTheme.Space.s)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("propose-amount-entry")
        .onChange(of: amountText) { _, value in
            model.setAmount(micros: AmountEntryText.micros(value) ?? 0)
        }
    }

    @ViewBuilder private var notices: some View {
        if model.isOverLimit {
            MonacoNotice(
                .error, message: model.trade.isSell ? ProposeFlowCopy.overHolding : ProposeFlowCopy.overCash)
        } else if let problem = model.message(assetName: stock.name) {
            MonacoNotice(.error, message: problem)
        }
        if model.previewFailed {
            MonacoErrorRow(thing: "the price", identifier: "propose-amount-price-error", inset: false) {
                model.retryPreview()
            }
        }
    }

    private var thesisSection: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text(ProposeFlowCopy.thesisTitle)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.ink)
            MonacoTextField(
                ProposeFlowCopy.thesisPlaceholder,
                text: Binding(get: { model.thesis }, set: { model.setThesis($0) }), isMultiline: true
            )
            .toolbar {
                ToolbarItemGroup(placement: .keyboard) {
                    Spacer()
                    Button("Done") {
                        UIApplication.shared.sendAction(
                            #selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
                    }
                    .accessibilityIdentifier("propose-amount-reason-done")
                }
            }
            .accessibilityIdentifier("propose-amount-reason")
            if ProposeReasonRules.showsCounter(for: model.thesis) {
                Text("\(model.thesis.count)/\(ProposeReasonRules.thesisLimit)")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .frame(maxWidth: .infinity, alignment: .trailing)
            }
            if model.showsThesisRequired {
                MonacoNotice(.error, message: ProposeFlowCopy.thesisRequired)
            }
        }
    }

    private var presets: [AmountPreset] {
        model.trade.isSell
            ? [.fraction(0.25, label: "25%"), .fraction(0.5, label: "50%"), .fraction(1, label: "All")]
            : [.dollars(25), .dollars(50), .dollars(100), .fraction(1, label: "Max")]
    }

    private var max: Decimal? { model.maxMicros.flatMap(ProposeMath.usd(fromMicros:)) }
}

extension ProposeContextDetail {
    static func cash(micros: Int64) -> ProposeContextDetail { .cash(micros: micros, failed: false) }
}
