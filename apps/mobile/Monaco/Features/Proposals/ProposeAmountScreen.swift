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

    init(
        service: MonacoCore.ProposeService, cabalID: String, stock: ProposeStock, potMicros: Int64 = 0,
        isSell: Bool = false
    ) {
        self.stock = stock
        self.cabalID = cabalID
        _model = State(
            initialValue: ProposeAmountModel(
                service: service, cabalID: cabalID, symbol: stock.symbol, potMicros: potMicros, isSell: isSell,
                clock: ContinuousClock()))
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                MonacoGroupedList { ProposeStockRow(stock: stock, logoURL: nil, isLast: true) }
                AmountEntry(amountText: $amountText, max: max, presets: presets, helper: helper)
                    .onChange(of: amountText) { _, value in model.setAmount(micros: AmountEntryText.micros(value) ?? 0)
                    }
                if !showsReason {
                    Button("+ Add a reason") { showsReason = true }.buttonStyle(.monacoSecondary)
                } else {
                    TextField(
                        model.isSell ? "Why should the cabal sell this?" : "Why should the cabal buy this?",
                        text: Binding(get: { model.thesis }, set: { model.thesis = $0 }), axis: .vertical
                    )
                    .textFieldStyle(.roundedBorder)
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
                    service: MonacoCore.LiveProposeService(api: environment.api), cabalID: cabalID, stock: stock,
                    draft: model.draft, preview: preview)
            }
        }
        .accessibilityIdentifier("propose-amount-screen")
    }

    private var presets: [AmountPreset] {
        [
            .dollars(25), .dollars(50), .dollars(100), .fraction(1, label: "Max"),
        ]
    }

    private var max: Decimal? { model.maxMicros.flatMap(ProposeMath.usd(fromMicros:)) }
    private var helper: String? { model.potHelperText }
}

#if DEBUG
final class ProposeAmountSampleHarnessEntry: SampleHarnessEntry {
    @MainActor override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains("-MonacoProposeAmountSample") else { return nil }
        return AnyView(NavigationStack { Text("Amount sample") })
    }
}
#endif
