import MonacoCore
import SwiftUI

struct ProposeAmountScreen: View {
    let stock: ProposeStock
    private let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @State private var showsReview = false
    @State private var model: ProposeAmountModel
    @State private var amountText = ""
    @State private var showsReason = false
    @FocusState private var reasonFocused: Bool

    init(service: MonacoCore.ProposeService, cabalID: String, stock: ProposeStock, trade: ProposeTrade? = nil) {
        self.stock = stock
        self.cabalID = cabalID
        let trade = trade ?? .buy(symbol: stock.symbol, kind: stock.assetKind, tokenDecimals: stock.tokenDecimals)
        _model = State(
            initialValue: ProposeAmountModel(
                service: service, cabalID: cabalID, trade: trade, clock: ContinuousClock()))
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    operationLabel
                    MonacoGroupedList { ProposeStockRow(stock: stock, logoURL: nil, isLast: true) }
                    ProposePotTotalRow(cabalID: cabalID)
                }
                AmountEntry(
                    amountText: $amountText, max: max, presets: presets, helper: model.helperText,
                    overLimitHelper: "More than the cabal holds", input: .keypad,
                    showsKeypad: !reasonFocused
                ) {
                    reasonSection
                }
                .onChange(of: amountText) { _, value in model.setAmount(micros: AmountEntryText.micros(value) ?? 0) }
                if let message = model.message(assetName: stock.name) {
                    Text(message).foregroundStyle(MonacoTheme.loss)
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.s)
            .padding(.bottom, MonacoTheme.Space.s)
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoCanvas()
        .task {
            guard !model.trade.isSell,
                let asset = try? await ProposalsRepository(api: environment.api).asset(symbol: stock.symbol)
            else { return }
            model.resolveAsset(kind: asset.kind, decimals: asset.decimals)
        }
        .navigationTitle("Amount")
        .navigationBarTitleDisplayMode(.inline)
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button("Review") { showsReview = true }.buttonStyle(.monacoPrimary)
                    .disabled(!model.reviewEnabled(assetName: stock.name))
                    .accessibilityIdentifier("propose-amount-review")
            }
        }
        .navigationDestination(isPresented: $showsReview) {
            if let preview = model.preview {
                ProposeReviewScreen(
                    service: MonacoCore.LiveProposeService(api: environment.api), cabalID: cabalID,
                    draft: model.draft, preview: preview, trade: model.trade)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("propose-amount-screen")
    }

    private var operationLabel: some View {
        let isSell = model.trade.isSell
        return Text(isSell ? "Sell" : "Buy")
            .font(MonacoTheme.Typo.captionStrong)
            .foregroundStyle(isSell ? MonacoTheme.lossOnWash : MonacoTheme.profitOnWash)
            .padding(.horizontal, MonacoTheme.Space.sm)
            .padding(.vertical, MonacoTheme.Space.xs)
            .background(isSell ? MonacoTheme.lossWash : MonacoTheme.profitWash, in: Capsule())
            .accessibilityIdentifier("propose-amount-operation")
    }

    @ViewBuilder
    private var reasonSection: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            if !showsReason {
                Button("Add a reason") { showsReason = true }.buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("propose-amount-add-reason")
            } else {
                reasonField
                if ProposeReasonRules.showsCounter(for: model.thesis) {
                    Text("\(model.thesis.count)/\(ProposeReasonRules.thesisLimit)")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                }
            }
        }
    }

    private var reasonField: some View {
        TextField(
            model.trade.isSell ? "Why should the cabal sell this?" : "Why should the cabal buy this?",
            text: Binding(get: { model.thesis }, set: { model.setThesis($0) }), axis: .vertical
        )
        .font(MonacoTheme.Typo.body)
        .foregroundStyle(MonacoTheme.ink)
        .tint(MonacoTheme.ink)
        .focused($reasonFocused)
        .monacoFieldChrome(isFocused: reasonFocused)
        .accessibilityIdentifier("propose-amount-reason")
    }

    private var presets: [AmountPreset] {
        model.trade.isSell
            ? [.fraction(0.25, label: "25%"), .fraction(0.5, label: "50%"), .fraction(1, label: "All")]
            : [.dollars(25), .dollars(50), .dollars(100), .fraction(1, label: "Max")]
    }

    private var max: Decimal? { model.maxMicros.flatMap(ProposeMath.usd(fromMicros:)) }
}

#if DEBUG
final class ProposeAmountSampleHarnessEntry: SampleHarnessEntry {
    @MainActor override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains("-MonacoProposeAmountSample") else { return nil }
        return AnyView(NavigationStack { Text("Amount sample") })
    }
}
#endif
