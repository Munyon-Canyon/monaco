import SwiftUI

private struct MonacoKeyRowsInsetKey: EnvironmentKey {
    static let defaultValue = false
}

extension EnvironmentValues {
    var monacoKeyRowsInset: Bool {
        get { self[MonacoKeyRowsInsetKey.self] }
        set { self[MonacoKeyRowsInsetKey.self] = newValue }
    }
}

nonisolated enum ReceiptLineMetrics {
    static let valueFraction: CGFloat = 0.63
    static let minHeight: CGFloat = 44
}

struct ReceiptLine: View {
    enum Value: Equatable {
        case data(String)
        case words(String)
        case address(String)
    }

    let label: String
    let value: Value
    var isLast = false

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.monacoKeyRowsInset) private var insideCard

    private var isStacked: Bool {
        if case .address = value { return true }
        return dynamicTypeSize.isAccessibilitySize
    }

    var body: some View {
        Group {
            if isStacked {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    labelText
                    valueView(alignment: .leading)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            } else {
                KeyRowLayout(spacing: MonacoTheme.Space.sm) {
                    labelText
                    valueView(alignment: .trailing)
                }
            }
        }
        .padding(.horizontal, insideCard ? 0 : MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: ReceiptLineMetrics.minHeight)
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule()
                    .padding(.horizontal, insideCard ? 0 : MonacoTheme.Space.gutter)
            }
        }
        .accessibilityElement(children: .combine)
    }

    private var labelText: some View {
        Text(label)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
    }

    @ViewBuilder
    private func valueView(alignment: TextAlignment) -> some View {
        switch value {
        case .data(let text):
            Text(text)
                .font(MonacoTheme.Typo.subheadStrong.monospacedDigit())
                .foregroundStyle(MonacoTheme.primaryText)
                .multilineTextAlignment(alignment)
                .monacoPrivate()
        case .words(let text):
            Text(text)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.primaryText)
                .multilineTextAlignment(alignment)
        case .address(let address):
            MonacoWalletAddressText(address: address, textStyle: .subheadline)
        }
    }
}

private struct KeyRowLayout: Layout {
    let spacing: CGFloat

    private func sizes(width: CGFloat, subviews: Subviews) -> (label: CGSize, value: CGSize) {
        let valueWidth = width * ReceiptLineMetrics.valueFraction
        let value = subviews[1].sizeThatFits(ProposedViewSize(width: valueWidth, height: nil))
        let labelWidth = max(width - value.width - spacing, 0)
        let label = subviews[0].sizeThatFits(ProposedViewSize(width: labelWidth, height: nil))
        return (label, value)
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        guard subviews.count == 2 else { return .zero }
        guard let width = proposal.width, width.isFinite else {
            let label = subviews[0].sizeThatFits(.unspecified)
            let value = subviews[1].sizeThatFits(.unspecified)
            return CGSize(width: label.width + spacing + value.width, height: max(label.height, value.height))
        }
        let measured = sizes(width: width, subviews: subviews)
        return CGSize(width: width, height: max(measured.label.height, measured.value.height))
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        guard subviews.count == 2 else { return }
        let measured = sizes(width: bounds.width, subviews: subviews)
        subviews[0].place(
            at: bounds.origin, anchor: .topLeading,
            proposal: ProposedViewSize(width: measured.label.width, height: bounds.height))
        subviews[1].place(
            at: CGPoint(x: bounds.maxX, y: bounds.minY), anchor: .topTrailing,
            proposal: ProposedViewSize(width: measured.value.width, height: bounds.height))
    }
}
