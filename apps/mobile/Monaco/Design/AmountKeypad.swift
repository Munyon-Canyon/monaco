import SwiftUI

struct AmountKeypad: View {
    @Binding var amountText: String
    @ScaledMetric(relativeTo: .title) private var rowHeight: CGFloat = 56

    private static let rows: [[AmountKey]] = [
        [.digit(1), .digit(2), .digit(3)],
        [.digit(4), .digit(5), .digit(6)],
        [.digit(7), .digit(8), .digit(9)],
        [.dot, .digit(0), .delete],
    ]

    var body: some View {
        Grid(horizontalSpacing: MonacoTheme.Space.xs, verticalSpacing: MonacoTheme.Space.xs) {
            ForEach(Array(Self.rows.enumerated()), id: \.offset) { _, row in
                GridRow {
                    ForEach(row, id: \.self) { key in
                        keyView(key)
                    }
                }
            }
        }
        .dynamicTypeSize(...DynamicTypeSize.accessibility1)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("amount-keypad")
    }

    private func keyView(_ key: AmountKey) -> some View {
        face(key)
            .frame(maxWidth: .infinity, minHeight: max(rowHeight, 44))
            .contentShape(Rectangle())
            .onTapGesture { press(key) }
            .onLongPressGesture(minimumDuration: 0.5) {
                guard key == .delete, !amountText.isEmpty else { return }
                Haptics.selection()
                amountText = ""
            }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(label(key))
            .accessibilityIdentifier(identifier(key))
            .accessibilityAddTraits(.isButton)
            .accessibilityAction(.default) { press(key) }
            .accessibilityActions {
                if key == .delete { Button("Clear") { amountText = "" } }
            }
    }

    @ViewBuilder
    private func face(_ key: AmountKey) -> some View {
        switch key {
        case .digit(let digit):
            Text(String(digit)).font(MonacoTheme.Typo.title).foregroundStyle(MonacoTheme.ink)
        case .dot:
            Text(".").font(MonacoTheme.Typo.title).foregroundStyle(MonacoTheme.ink)
        case .delete:
            Image(systemName: "delete.left").font(MonacoTheme.Typo.section).foregroundStyle(MonacoTheme.ink)
        }
    }

    private func press(_ key: AmountKey) {
        Haptics.selection()
        amountText = AmountEntryText.applying(key, to: amountText)
    }

    private func label(_ key: AmountKey) -> String {
        switch key {
        case .digit(let digit): return String(digit)
        case .dot: return "Decimal point"
        case .delete: return "Delete"
        }
    }

    private func identifier(_ key: AmountKey) -> String {
        switch key {
        case .digit(let digit): return "amount-keypad-\(digit)"
        case .dot: return "amount-keypad-dot"
        case .delete: return "amount-keypad-delete"
        }
    }
}
