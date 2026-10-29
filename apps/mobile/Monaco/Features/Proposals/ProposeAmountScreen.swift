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
    private let onChangeCabal: ((Int64) -> Void)?
    @Environment(AppEnvironment.self) private var environment
    @State private var showsReview = false
    @State private var model: ProposeAmountModel
    @State private var reviewMemory = MonacoCore.ProposeReviewMemory()
    @State private var amountText = ""
    @State private var context = MonacoCore.ProposeCabalContext()

    init(
        service: MonacoCore.ProposeService, cabalID: String, stock: ProposeStock, trade: ProposeTrade? = nil,
        bestChains: AssetBestChainStore? = nil, initialAmountMicros: Int64? = nil,
        onChangeCabal: ((Int64) -> Void)? = nil
    ) {
        self.stock = stock
        self.cabalID = cabalID
        self.onChangeCabal = onChangeCabal
        if let micros = initialAmountMicros, micros > 0 {
            _amountText = State(initialValue: "\(ProposeMath.usd(fromMicros: micros))")
        }
        let trade = trade ?? .buy(symbol: stock.symbol, kind: stock.assetKind, tokenDecimals: stock.tokenDecimals)
        _model = State(
            initialValue: ProposeAmountModel(
                service: service, cabalID: cabalID, trade: trade, clock: ContinuousClock(), bestChains: bestChains,
                initialAmountMicros: initialAmountMicros))
    }

    static let markSize: CGFloat = 32

    private var displayName: String { AssetDisplayName.format(catalogName: stock.name, kind: stock.assetKind) }
    private var operation: String { model.trade.isSell ? "Sell" : "Buy" }

    var body: some View {
        Group {
            if !model.trade.isSell, model.cashFailed {
                EmptyState.loadFailed(identifier: "propose-amount-cash-error") { Task { await model.load() } }
                    .frame(maxHeight: .infinity, alignment: .top)
            } else {
                keypad
            }
        }
        .monacoCanvas()
        .task { await model.load() }
        .task { await model.prefetchBestChain() }
        .task { await applySeed() }
        .task { await context.load(api: environment.api, cabalID: cabalID) }
        .task {
            guard !model.trade.isSell,
                let asset = try? await ProposalsRepository(api: environment.api).asset(symbol: stock.symbol)
            else { return }
            model.resolveAsset(kind: asset.kind, decimals: asset.decimals)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("propose-amount-screen")
    }

    private var keypad: some View {
        AmountKeypadScreen(
            amountText: $amountText,
            state: AmountKeypadState(model.keypadState),
            line: (model.keypadLine ?? model.message(assetName: stock.name)).map { AmountKeypadLine(text: $0) },
            presets: presets,
            max: max,
            action: AmountKeypadAction(title: model.reviewTitle, isWorking: model.isLoading) {
                if model.previewFailed { model.retryPreview() } else { review() }
            },
            onClose: { environment.navigator.closeProposeFlow(in: environment.navigator.selectedTab) },
            onSource: onChangeCabal.map { change in { change(model.amountMicros) } },
            source: { sourcePill },
            header: { header }
        )
        .onChange(of: amountText) { _, value in
            model.setAmount(micros: AmountText.micros(value) ?? 0)
        }
        .sheet(isPresented: $showsReview) {
            if let preview = model.preview {
                reviewSheet(preview)
            }
        }
    }

    @ViewBuilder private var sourcePill: some View {
        CabalMark(
            groupId: cabalID, name: context.name ?? "", size: Self.markSize, pictureUrl: context.pictureURL)
        Text(context.name ?? " ")
            .font(MonacoTheme.Typo.headline)
            .foregroundStyle(MonacoTheme.primaryText)
            .monacoPrivate()
        Text(sourceDetail)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .monacoPrivate()
    }

    private var sourceDetail: String {
        if case .sell(let holding) = model.trade { return "\(holding.heldQuantity) held" }
        return model.spendableCashMicros.map(FundSourceCopy.cash(micros:)) ?? ""
    }

    private var header: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            StockMark(
                symbol: stock.symbol, displayName: stock.name, assetKind: stock.assetKind, size: Self.markSize,
                logoURL: stock.logoURL)
            Text("\(operation) \(displayName)").font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
        }
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(operation) \(displayName)")
        .accessibilityAddTraits(.isHeader)
    }

    private func reviewSheet(_ preview: MonacoCore.ProposePreview) -> ProposeReviewSheet {
        var sheet = ProposeReviewSheet(
            service: MonacoCore.LiveProposeService(api: environment.api), cabalID: cabalID,
            draft: model.draft, preview: preview, trade: model.trade,
            buyName: displayName, bestChain: model.previewBestChain, logoURL: stock.logoURL,
            memory: reviewMemory, sendsOnAppear: seed.sendsReview)
        if let onChangeCabal {
            sheet.onChangeCabal = {
                showsReview = false
                onChangeCabal(model.amountMicros)
            }
        }
        return sheet
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
        if model.reviewEnabled(assetName: stock.name) { showsReview = true }
    }

    private var presets: [AmountPreset] {
        model.trade.isSell
            ? [.fraction(0.25, label: "25%"), .fraction(0.5, label: "50%"), .fraction(1, label: "All")]
            : [.dollars(25), .dollars(50), .dollars(100), .fraction(1, label: "Max")]
    }

    private var max: Decimal? { model.maxMicros.flatMap(ProposeMath.usd(fromMicros:)) }
}

extension AmountKeypadState {
    init(_ value: AmountKeypadStateValue) {
        switch value {
        case .empty: self = .empty
        case .typed: self = .typed
        case .invalid(let limitLine, let slot): self = .invalid(limitLine: limitLine, slot: slot)
        case .settled(let line): self = .settled(line: line)
        }
    }
}
