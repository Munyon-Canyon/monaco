import MonacoCore
import SwiftUI

struct ProposalHero: View {
    let summary: ProposalSummary
    let asset: ProposalAsset?
    let members: [ProposalMember]
    let paused: Bool
    var subtitle: String?

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var state: ProposalState { summary.state(now: .now) }
    private var proposer: ProposalMember? { members.first { $0.id == summary.proposerID } }
    private var cabalLabel: String { summary.cabalName.isEmpty ? "Your cabal" : summary.cabalName }
    private var kindLabel: String { summary.kind == "sell" ? "Sell" : "Buy" }
    private var ticker: String {
        asset?.kind == .preIpo
            ? summary.assetName(asset: asset)
            : AssetSymbolFormatter.display(summary.symbol, kind: asset?.kind ?? .stock)
    }

    static func crestLine(
        cabalName: String, proposer: String, createdAt: Date, now: Date, calendar: Calendar
    ) -> String {
        let age = ProposalStats.crestAge(since: createdAt, now: now, calendar: calendar)
        return "\(cabalName) · \(proposer) proposed · \(age)"
    }

    static func amountLabel(micros: Int64) -> String {
        micros % 1_000_000 == 0
            ? UsdAmountFormatter.format(micros: micros).replacingOccurrences(of: ".00", with: "")
            : UsdAmountFormatter.format(micros: micros)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            crest
            heroRow
            if paused && state.isOpen {
                Text(ProposalCardCopy.pausedCaption(isSell: summary.kind == "sell"))
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("proposal-paused-caption")
            }
        }
    }

    private var crest: some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: summary.cabalID))) {
            HStack(spacing: MonacoTheme.Space.s) {
                CabalMark(groupId: summary.cabalID, name: cabalLabel, size: 20)
                Text(
                    Self.crestLine(
                        cabalName: cabalLabel, proposer: proposer?.name ?? "Member", createdAt: summary.createdAt,
                        now: .now, calendar: .current)
                )
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 1)
                .fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: 0)
            }
            .frame(minHeight: MonacoButtonMetrics.hitTarget, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("proposal-crest-line")
    }

    @ViewBuilder private var heroRow: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                mark
                VStack(alignment: .leading, spacing: 0) {
                    title
                    subline
                }
                amount
            }
            .accessibilityElement(children: .combine)
        } else {
            HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
                mark
                VStack(alignment: .leading, spacing: 0) {
                    HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                        title
                        Spacer(minLength: MonacoTheme.Space.s)
                        amount
                    }
                    subline
                }
            }
            .accessibilityElement(children: .combine)
        }
    }

    private var mark: some View {
        StockMark(
            symbol: summary.symbol, displayName: asset?.displayName, assetKind: asset?.kind ?? .stock,
            size: .review, logoURL: asset?.logoURL)
    }

    private var title: some View {
        Text("\(kindLabel) \(ticker)")
            .font(MonacoTheme.Typo.title)
            .foregroundStyle(MonacoTheme.ink)
            .fixedSize(horizontal: false, vertical: true)
    }

    @ViewBuilder private var amount: some View {
        if let micros = amountMicros {
            Text(Self.amountLabel(micros: micros))
                .moneyFigure(.card)
                .foregroundStyle(MonacoTheme.ink)
                .accessibilityIdentifier("proposal-hero-amount")
        }
    }

    @ViewBuilder private var subline: some View {
        let isAccessibility = dynamicTypeSize.isAccessibilitySize
        let line = Text(subtitle ?? summary.assetName(asset: asset))
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .lineLimit(isAccessibility ? nil : 1)
            .minimumScaleFactor(isAccessibility ? 1 : 0.75)
            .fixedSize(horizontal: false, vertical: isAccessibility)
        if subtitle != nil {
            line.accessibilityIdentifier("proposal-hero-subtitle")
        } else {
            line
        }
    }

    private var amountMicros: Int64? {
        if summary.kind == "sell" { return summary.quoteOutAmount > 0 ? summary.quoteOutAmount : nil }
        return summary.usdcMicros
    }
}
