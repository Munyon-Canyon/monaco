import MonacoCore
import SwiftUI

struct BestPriceSection: View {
    let bestPrice: BestPrice
    @State private var showsVenues = false

    static func figure(_ bestPrice: BestPrice) -> String { bestPrice.price ?? bestPrice.figure(bestPrice.winner) }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Price")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoGroupedList {
                Button {
                    showsVenues = true
                } label: {
                    HStack(spacing: MonacoTheme.Space.sm) {
                        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                            Text(Self.figure(bestPrice))
                                .moneyFont(.row)
                                .foregroundStyle(MonacoTheme.ink)
                                .accessibilityIdentifier("best-price-figure")
                            Text(bestPrice.headline)
                                .font(MonacoTheme.Typo.subhead)
                                .foregroundStyle(MonacoTheme.secondaryText)
                                .accessibilityIdentifier("best-price-headline")
                        }
                        Spacer(minLength: MonacoTheme.Space.sm)
                        Image(systemName: "chevron.right")
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.secondaryText)
                            .accessibilityHidden(true)
                    }
                    .padding(MonacoTheme.Space.m)
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.monacoRow)
                .accessibilityElement(children: .combine)
                .accessibilityHint("Shows every venue checked")
                .accessibilityIdentifier("best-price-row")
            }
        }
        .sheet(isPresented: $showsVenues) {
            BestPriceSheet(bestPrice: bestPrice)
        }
    }
}

struct BestPriceSheet: View {
    struct Row: Equatable, Identifiable {
        let id: String
        let name: String
        let figure: String
        let isWinner: Bool
    }

    let bestPrice: BestPrice
    @Environment(\.dismiss) private var dismiss

    static func rows(_ bestPrice: BestPrice) -> [Row] {
        bestPrice.venues.map {
            Row(id: $0.id, name: $0.name, figure: bestPrice.figure($0), isWinner: $0.isWinner)
        }
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
        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.sm) {
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
                .font(MonacoTheme.Typo.dataStrong)
                .foregroundStyle(MonacoTheme.ink)
        }
        .padding(MonacoTheme.Space.m)
        .overlay(alignment: .bottom) { if !isLast { MonacoRule() } }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(row.isWinner ? "best-price-winner" : "best-price-venue")
    }
}
