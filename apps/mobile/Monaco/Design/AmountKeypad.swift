import MonacoCore
import SwiftUI

struct AmountKeypad: View {
    @Binding var amountText: String

    var body: some View {
        AmountKeypadGrid(amountText: $amountText)
            .dynamicTypeSize(...DynamicTypeSize.accessibility1)
    }
}

enum AmountKeypadMetrics {
    static let keyHeight: CGFloat = 60
    static let keyFont = Font.largeTitle
    static let deleteFont = Font.title3
    static let pressedInset: CGFloat = MonacoTheme.Space.xs
}

private struct AmountKeypadHeldKeyKey: EnvironmentKey {
    static let defaultValue: AmountKey? = nil
}

extension EnvironmentValues {
    var amountKeypadHeldKey: AmountKey? {
        get { self[AmountKeypadHeldKeyKey.self] }
        set { self[AmountKeypadHeldKeyKey.self] = newValue }
    }
}

private struct AmountKeypadGrid: View {
    @Binding var amountText: String
    @ScaledMetric(relativeTo: .title) private var rowHeight: CGFloat = AmountKeypadMetrics.keyHeight
    @State private var pressedKey: AmountKey?
    @Environment(\.isEnabled) private var isEnabled
    @Environment(\.amountKeypadHeldKey) private var heldKey

    private static let rows: [[AmountKey]] = [
        [.digit(1), .digit(2), .digit(3)],
        [.digit(4), .digit(5), .digit(6)],
        [.digit(7), .digit(8), .digit(9)],
        [.dot, .digit(0), .delete],
    ]

    var body: some View {
        Grid(horizontalSpacing: MonacoTheme.Space.s, verticalSpacing: 0) {
            ForEach(Array(Self.rows.enumerated()), id: \.offset) { _, row in
                GridRow {
                    ForEach(row, id: \.self) { key in
                        keyView(key)
                    }
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("amount-keypad")
    }

    private func keyView(_ key: AmountKey) -> some View {
        face(key)
            .frame(maxWidth: .infinity, minHeight: max(rowHeight, MonacoButtonMetrics.hitTarget))
            .background(
                Capsule(style: .continuous)
                    .fill((pressedKey == key || heldKey == key) && isEnabled ? MonacoTheme.surfaceSunken : Color.clear)
                    .padding(.vertical, AmountKeypadMetrics.pressedInset)
            )
            .contentShape(Rectangle())
            .onTapGesture { press(key) }
            .onLongPressGesture(
                minimumDuration: 0.5,
                perform: {
                    guard isEnabled, key == .delete, !amountText.isEmpty else { return }
                    Haptics.selection()
                    amountText = ""
                },
                onPressingChanged: { pressed in
                    if pressed, isEnabled {
                        pressedKey = key
                    } else if pressedKey == key {
                        pressedKey = nil
                    }
                }
            )
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(label(key))
            .accessibilityIdentifier(identifier(key))
            .accessibilityAddTraits(.isButton)
            .accessibilityAction(.default) { press(key) }
            .accessibilityActions {
                if key == .delete, isEnabled { Button("Clear") { amountText = "" } }
            }
    }

    @ViewBuilder
    private func face(_ key: AmountKey) -> some View {
        switch key {
        case .digit(let digit):
            Text(String(digit)).font(AmountKeypadMetrics.keyFont).foregroundStyle(
                isEnabled ? MonacoTheme.primaryText : MonacoTheme.disabledLabel)
        case .dot:
            Text(".").font(AmountKeypadMetrics.keyFont).foregroundStyle(
                isEnabled ? MonacoTheme.primaryText : MonacoTheme.disabledLabel)
        case .delete:
            Image(systemName: "delete.left").font(AmountKeypadMetrics.deleteFont).foregroundStyle(
                isEnabled ? MonacoTheme.primaryText : MonacoTheme.disabledLabel)
        }
    }

    private func press(_ key: AmountKey) {
        guard isEnabled else { return }
        Haptics.selection()
        amountText = AmountText.applying(key, to: amountText)
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
