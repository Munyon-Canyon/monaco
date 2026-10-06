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
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                MonacoGroupedList { ProposeStockRow(stock: stock, logoURL: nil, isLast: true) }
                AmountEntry(
                    amountText: $amountText, max: max, presets: presets, helper: model.helperText,
                    overLimitHelper: "More than the cabal holds"
                )
                .onChange(of: amountText) { _, value in model.setAmount(micros: AmountEntryText.micros(value) ?? 0) }
                if !showsReason {
                    Button("+ Add a reason") { showsReason = true }.buttonStyle(.monacoSecondary)
                        .accessibilityIdentifier("propose-amount-add-reason")
                } else {
                    TextField(
                        model.trade.isSell ? "Why should the cabal sell this?" : "Why should the cabal buy this?",
                        text: Binding(get: { model.thesis }, set: { model.setThesis($0) }), axis: .vertical
                    )
                    .textFieldStyle(.roundedBorder)
                    .accessibilityIdentifier("propose-amount-reason")
                    if ProposeReasonRules.showsCounter(for: model.thesis) {
                        Text("\(model.thesis.count)/\(ProposeReasonRules.thesisLimit)").foregroundStyle(
                            MonacoTheme.muted)
                    }
                }
                if let message = model.message(assetName: stock.name) {
                    Text(message).foregroundStyle(MonacoTheme.loss)
                }
            }
            .padding(MonacoTheme.Space.m)
        }
        .monacoCanvas()
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
