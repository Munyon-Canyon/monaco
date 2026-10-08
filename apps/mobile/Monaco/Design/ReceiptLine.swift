import SwiftUI

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
                HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.sm) {
                    labelText
                    Spacer(minLength: MonacoTheme.Space.sm)
                    valueView(alignment: .trailing)
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule()
                    .padding(.leading, MonacoTheme.Space.gutter)
            }
        }
        .accessibilityElement(children: .combine)
    }

    private var labelText: some View {
        Text(label)
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.muted)
    }

    @ViewBuilder
    private func valueView(alignment: TextAlignment) -> some View {
        switch value {
        case .data(let text):
            Text(text)
                .font(MonacoTheme.Typo.data)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(alignment)
        case .words(let text):
            Text(text)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(alignment)
        case .address(let address):
            MonacoWalletAddressText(address: address, textStyle: .subheadline)
        }
    }
}
