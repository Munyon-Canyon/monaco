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
#endif
