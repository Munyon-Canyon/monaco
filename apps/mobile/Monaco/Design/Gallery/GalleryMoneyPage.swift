#if DEBUG
import MonacoCore
import SwiftUI

struct GalleryMoneyPage: View {
    @State private var ticking: Int64 = 1_248_500_000

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                MoneyHero(
                    label: "In cabals", value: UsdAmountFormatter.format(micros: ticking), dollarChange: "+48.20",
                    percentChange: "0.096")
                MoneyHero(label: "Pot", value: "$0.00")
                MoneyHero(label: "Amount", value: "$50", alignment: .center)

                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    MonacoSectionHeader("Hero")
                    MoneyText(micros: 0, style: .hero)
                    MoneyText(decimalString: "1248.50", style: .hero)
                    MoneyText(decimalString: "12431180", style: .hero)
                    MoneyText(decimalString: "not a number", style: .hero)
                }

                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    MonacoSectionHeader("Sizes")
                    MoneyText(micros: 548_200_000, style: .large)
                    MoneyText(micros: 548_200_000, style: .row)
                    MoneyText(micros: 548_200_000, style: .caption)
                }

                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    MonacoSectionHeader("Profit and loss")
                    GalleryGrid(minimum: 90) {
                        PnLText(dollarPnl: "+48.20", style: .row)
                        PnLText(dollarPnl: "-7.60", style: .row)
                        PnLText(dollarPnl: "+0.00", style: .row)
                        PnLText(dollarPnl: "-0.00", style: .row)
                    }
                    GalleryGrid(minimum: 90) {
                        PercentText(percentReturn: "0.096", style: .row)
                        PercentText(percentReturn: "-0.036", style: .row)
                        PercentText(percentReturn: "0", style: .row)
                        PercentText(percentReturn: nil, style: .row)
                    }
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                        PnLBadge(dollarPnl: "+48.20", percentReturn: "0.096")
                        PnLBadge(dollarPnl: "-7.60", percentReturn: "-0.036")
                        PnLBadge(dollarPnl: "-0.001", percentReturn: "0")
                        PnLBadge(dollarPnl: "+311.50", percentReturn: nil, style: .row)
                    }
                }

                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    MonacoSectionHeader("Keypad")
                    ForEach(GalleryKeypadSample.all) { sample in
                        ForEach([ColorScheme.dark, .light], id: \.self) { scheme in
                            GalleryKeypadFrame(sample: sample, scheme: scheme)
                        }
                    }
                }

                Button("Change the hero figure") {
                    withAnimation(.snappy) { ticking += Int64.random(in: -80_000_000...120_000_000) / 10_000 * 10_000 }
                }
                .buttonStyle(.monacoSecondary)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .monacoCanvas()
        .navigationTitle("")
        .navigationBarTitleDisplayMode(.inline)
    }
}

private struct GalleryKeypadSample: Identifiable {
    let id: String
    let text: String
    let state: AmountKeypadState
    let line: AmountKeypadLine?
    let sourceName: String
    let balance: String
    let hasHeader: Bool

    private static let fundLine = AmountKeypadLine(text: "From your cash · your share 35% → 36%", info: {})

    static let all: [GalleryKeypadSample] = [
        GalleryKeypadSample(
            id: "Empty", text: "", state: .empty,
            line: AmountKeypadLine(text: "From your cash · your share 35%", info: {}),
            sourceName: "AC Capital", balance: "Cash $500.00", hasHeader: false),
        GalleryKeypadSample(
            id: "Typed", text: "250", state: .typed, line: fundLine,
            sourceName: "AC Capital", balance: "Cash $500.00", hasHeader: false),
        GalleryKeypadSample(
            id: "Invalid", text: "650",
            state: .invalid(limitLine: "$500.00 available · $150.00 over", slot: "Up to $500.00"),
            line: AmountKeypadLine(text: "From your cash · your share 35% → 41%", info: {}),
            sourceName: "AC Capital", balance: "Cash $500.00", hasHeader: false),
        GalleryKeypadSample(
            id: "Settled", text: "250", state: .settled(line: "Added just now · your share is now 36%"),
            line: nil, sourceName: "AC Capital", balance: "Cash $500.00", hasHeader: false),
        GalleryKeypadSample(
            id: "Invalid with a header", text: "650",
            state: .invalid(limitLine: "You can send $499.50 · $150.50 over", slot: "Up to $499.50"),
            line: AmountKeypadLine(text: "≈ 650.00 USDC · fee $0.50", info: {}),
            sourceName: "Cash", balance: "$500.00", hasHeader: true),
    ]
}

private struct GalleryKeypadFrame: View {
    let sample: GalleryKeypadSample
    let scheme: ColorScheme
    @State private var text: String

    init(sample: GalleryKeypadSample, scheme: ColorScheme) {
        self.sample = sample
        self.scheme = scheme
        _text = State(initialValue: sample.text)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text("\(sample.id) · \(scheme == .dark ? "dark" : "light")")
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
            screen
                .frame(height: 760)
                .padding(.horizontal, -MonacoTheme.Space.gutter)
                .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                        .strokeBorder(MonacoTheme.hairline, lineWidth: 1)
                )
                .environment(\.colorScheme, scheme)
        }
    }

    private var screen: some View {
        AmountKeypadScreen(
            amountText: $text, state: sample.state, line: sample.line, max: 500,
            action: AmountKeypadAction(title: "Fund $250", settledTitle: "$250 to AC Capital", perform: {}),
            onClose: {},
            source: {
                if !sample.hasHeader {
                    CabalMark(groupId: GalleryFixtures.cabalNames[0].id, name: sample.sourceName, size: 32)
                } else {
                    Image(systemName: "creditcard")
                }
                Text(sample.sourceName).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
                Text(sample.balance).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
            },
            header: {
                if sample.hasHeader {
                    Text("Address and network chips go here")
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .frame(maxWidth: .infinity, minHeight: 96)
                        .background(
                            MonacoTheme.surfaceSunken, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
                }
            })
    }
}
#endif
