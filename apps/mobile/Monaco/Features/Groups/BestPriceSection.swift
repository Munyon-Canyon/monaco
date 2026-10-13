import MonacoAPI
import MonacoCore
import SwiftUI

struct BestPriceSection: View {
    let row: BestPriceRow
    @State private var showsVenues = false

    init(row: BestPriceRow) { self.row = row }

    init(bestPrice: BestPrice) { row = BestPriceRow(bestPrice) }

    static func figure(_ bestPrice: BestPrice) -> String { BestPriceRow(bestPrice).figure }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Price")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoGroupedList {
                if row.venues != nil {
                    Button {
                        showsVenues = true
                    } label: {
                        label(chevron: true)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityElement(children: .combine)
                    .accessibilityHint("Shows every venue checked")
                    .accessibilityIdentifier("best-price-row")
                } else {
                    label(chevron: false)
                        .accessibilityElement(children: .combine)
                        .accessibilityIdentifier("best-price-row")
                }
            }
        }
        .sheet(isPresented: $showsVenues) {
            if let venues = row.venues { BestPriceSheet(bestPrice: venues) }
        }
    }

    private func label(chevron: Bool) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            ChainMark(network: row.network.rawValue)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(row.figure)
                    .moneyFont(.row)
                    .foregroundStyle(MonacoTheme.ink)
                    .accessibilityIdentifier("best-price-figure")
                if let headline = row.headline {
                    Text(headline)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .accessibilityIdentifier("best-price-headline")
                }
                if let caption = row.caption {
                    Text(caption)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .accessibilityIdentifier("best-price-age")
                }
            }
            Spacer(minLength: MonacoTheme.Space.sm)
            if chevron {
                Image(systemName: "chevron.right")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
            }
        }
        .padding(MonacoTheme.Space.m)
        .frame(minHeight: 44)
        .contentShape(Rectangle())
    }
}

struct BestPriceSheet: View {
    struct Row: Equatable, Identifiable {
        let id: String
        let name: String
        let network: String
        let figure: String
        let isWinner: Bool
        var isUnavailable = false
    }

    let bestPrice: BestPrice
    @Environment(\.dismiss) private var dismiss

    static func rows(_ bestPrice: BestPrice) -> [Row] {
        let quoted = bestPrice.venues.map {
            Row(
                id: $0.id, name: $0.name, network: $0.network.rawValue, figure: bestPrice.figure($0),
                isWinner: $0.isWinner)
        }
        let unavailable = bestPrice.unavailable.map {
            Row(
                id: $0.id, name: $0.name, network: $0.network.rawValue, figure: $0.reason, isWinner: false,
                isUnavailable: true)
        }
        return quoted + unavailable
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                    if let advantage = bestPrice.advantage {
                        Text(advantage)
                            .font(MonacoTheme.Typo.body)
                            .foregroundStyle(MonacoTheme.ink)
                            .fixedSize(horizontal: false, vertical: true)
                            .padding(.horizontal, MonacoTheme.Space.gutter)
                            .accessibilityIdentifier("best-price-advantage")
                    }
                    MonacoGroupedList {
                        let rows = Self.rows(bestPrice)
                        ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                            BestPriceVenueRow(row: row, isLast: index == rows.count - 1)
                        }
                    }
                }
                .padding(.vertical, MonacoTheme.Space.m)
            }
            .monacoCanvas()
            .navigationTitle(bestPrice.headline)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("best-price-done")
                }
            }
        }
        .presentationDetents([.medium, .large])
        .accessibilityIdentifier("best-price-sheet")
    }
}

private struct BestPriceVenueRow: View {
    let row: BestPriceSheet.Row
    let isLast: Bool

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            ChainMark(network: row.network, size: 32)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(row.name)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.ink)
                if row.isWinner {
                    Text("Best price")
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.profit)
                }
            }
            Spacer(minLength: MonacoTheme.Space.sm)
            Text(row.figure)
                .font(row.isUnavailable ? MonacoTheme.Typo.subhead : MonacoTheme.Typo.dataStrong)
                .foregroundStyle(row.isUnavailable ? MonacoTheme.secondaryText : MonacoTheme.ink)
        }
        .padding(MonacoTheme.Space.m)
        .opacity(row.isUnavailable ? 0.5 : 1)
        .overlay(alignment: .bottom) { if !isLast { MonacoRule() } }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(identifier)
    }

    private var identifier: String {
        if row.isUnavailable { return "best-price-unavailable" }
        return row.isWinner ? "best-price-winner" : "best-price-venue"
    }
}
