import MonacoCore
import SwiftUI

struct MarketStatsSheet: View {
    let facts: MarketStatsFacts

    var body: some View {
        let sheet = MarketStatsCopy.sheet(facts, now: .now)
        MonacoInfoSheet(
            title: sheet.title, message: sheet.message,
            rows: sheet.rows.map { MonacoInfoRow(label: $0.label, value: $0.value) },
            identifier: "market-stats-sheet")
    }
}

struct EstimateInfoSheet: View {
    let preIpo: PreIpoDetail

    var body: some View {
        let sheet = MarketStatsCopy.estimateSheet(preIpo)
        MonacoInfoSheet(
            title: sheet.title, message: sheet.message,
            rows: sheet.rows.map { MonacoInfoRow(label: $0.label, value: $0.value) },
            identifier: "estimate-info-sheet")
    }
}
