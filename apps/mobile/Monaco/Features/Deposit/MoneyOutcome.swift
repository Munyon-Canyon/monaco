import MonacoCore
import SwiftUI

typealias MoneyOutcomeTone = OutcomeView<EmptyView>.Tone

struct MoneyKeyRow: Equatable {
    let label: String
    let value: ReceiptLine.Value
}

struct MoneyKeyRowCard: View {
    let rows: [MoneyKeyRow]

    var body: some View {
        MonacoCard(style: .inset) {
            VStack(spacing: 0) {
                ForEach(Array(rows.enumerated()), id: \.offset) { index, row in
                    ReceiptLine(label: row.label, value: row.value, isLast: index == rows.count - 1)
                }
            }
        }
    }
}

struct MoneyOutcome<Footer: View>: View {
    let tone: MoneyOutcomeTone
    let title: String
    let message: String
    var rows: [MoneyKeyRow] = []
    var notice: String?
    var identifier: String
    @ViewBuilder let footer: Footer

    var body: some View {
        ScrollView {
            VStack(spacing: MonacoTheme.Space.l) {
                OutcomeView(tone: tone, title: title, message: message)
                if !rows.isEmpty {
                    MoneyKeyRowCard(rows: rows)
                }
                if let notice {
                    MonacoNotice(.warn, message: notice)
                }
                VStack(spacing: MonacoTheme.Space.sm) {
                    footer
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xl)
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoCanvas()
        .accessibilityIdentifier(identifier)
    }
}

enum MoneyCashLine {
    static func text(availableMicros: Int64, inFlightMicros: Int64) -> String {
        let cash = "Cash \(UsdAmountFormatter.format(flooredMicros: availableMicros))"
        guard inFlightMicros > 0 else { return cash }
        return "\(cash) · \(UsdAmountFormatter.format(micros: inFlightMicros)) pending"
    }
}

struct MoneyFormFooter<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            content
        }
        .padding(.top, MonacoTheme.Space.l)
        .padding(.bottom, MonacoTheme.Space.sm)
    }
}

struct MoneyLoadFailure: View {
    let identifier: String
    let retry: () -> Void

    var body: some View {
        EmptyState(
            title: "Couldn't load this view",
            message: "Your existing information hasn't been changed. Try again when your connection is available.",
            actionTitle: "Try again",
            actionIdentifier: identifier,
            systemImage: "wifi.exclamationmark",
            action: retry
        )
    }
}
