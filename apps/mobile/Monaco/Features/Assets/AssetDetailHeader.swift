import MonacoAPI
import MonacoCore
import SwiftUI

struct AssetDetailHeader: View {
    let detail: AssetDetailPresentation
    let model: AssetDetailClientModel
    let scrub: AssetScrubHeader?
    let priceTick: MonacoPriceTick?

    private static let logoSize: CGFloat = 40

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            summary
            notices
            session
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var summary: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            identity
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .lastTextBaseline, spacing: MonacoTheme.Space.sm) {
                    price
                    change
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    price
                    change
                }
            }
        }
        .padding(.top, MonacoTheme.Space.xs)
        .padding(.bottom, MonacoTheme.Space.m)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spokenSummary(detail: detail, scrub: scrub))
        .accessibilityIdentifier("asset-detail-header")
    }

    static func spokenSummary(detail: AssetDetailPresentation, scrub: AssetScrubHeader?) -> String {
        if let scrub { return "\(detail.name), \(scrub.price), \(scrub.caption)" }
        guard let move = detail.todayMove else {
            return detail.priceMicros.map { "\(detail.name), \(UsdAmountFormatter.format(micros: $0))" } ?? detail.name
        }
        return "\(detail.name), \(move.speech(priceMicros: detail.priceMicros))"
    }

    private var identity: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            StockMark(
                symbol: detail.symbol, displayName: detail.name, assetKind: detail.kind, size: Self.logoSize,
                logoURL: detail.market.logoURL)
            Text(detail.name)
                .font(MonacoTheme.Typo.title)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(2)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("asset-detail-name")
            Spacer(minLength: 0)
        }
    }

    @ViewBuilder private var notices: some View {
        let shown = model.notices
        if !shown.isEmpty {
            VStack(spacing: MonacoTheme.Space.s) {
                ForEach(Array(shown.enumerated()), id: \.offset) { _, notice in
                    MonacoNotice(.warn, message: notice.text)
                }
            }
            .padding(.bottom, MonacoTheme.Space.m)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("asset-detail-notices")
        }
    }

    @ViewBuilder private var price: some View {
        if let scrub {
            Text(scrub.price)
                .moneyFont(.hero)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .accessibilityIdentifier("asset-detail-price")
        } else if let micros = detail.priceMicros {
            Self.hero(UsdAmountFormatter.format(micros: micros))
                .moneyFont(.hero)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .priceTickFlash(
                    priceTick, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous),
                    expand: MonacoTheme.Space.xs
                )
                .accessibilityIdentifier("asset-detail-price")
        } else {
            Text("—")
                .moneyFont(.hero)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
        }
    }

    static func heroParts(_ text: String) -> (whole: String, cents: String) {
        guard let point = text.lastIndex(of: ".") else { return (text, "") }
        return (String(text[..<point]), String(text[point...]))
    }

    private static func hero(_ text: String) -> Text {
        let parts = heroParts(text)
        return Text(verbatim: parts.whole) + Text(verbatim: parts.cents).foregroundStyle(MonacoTheme.secondaryText)
    }

    @ViewBuilder private var change: some View {
        if let scrub {
            scrubLine(basisPoints: scrub.basisPoints, caption: scrub.caption)
        } else if let move = detail.todayMove {
            Text(move.line)
                .font(MonacoTheme.Typo.caption.monospacedDigit())
                .foregroundStyle(move.isLoss ? MonacoTheme.loss : MonacoTheme.profit)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
                .accessibilityIdentifier("asset-detail-range-change")
        }
    }

    private func scrubLine(basisPoints: Int64?, caption: String) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            if let basisPoints { PercentText(basisPoints: basisPoints, style: .row) }
            Text(caption).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
        }
        .accessibilityIdentifier("asset-detail-range-change")
    }

    @ViewBuilder private var session: some View {
        if detail.market.showsSessionChip, let copy = MarketSessionCopy.chip(for: detail.market.status), !copy.isLive {
            MarketSessionChip(session: copy)
                .accessibilityIdentifier("asset-detail-session")
                .padding(.bottom, MonacoTheme.Space.s)
        }
    }
}
