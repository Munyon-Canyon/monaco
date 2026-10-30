import MonacoCore
import SwiftUI

struct MonacoInfoRow: Equatable {
    let label: String
    let value: String
}

struct MonacoInfoSheet: View {
    let title: String
    let message: String
    let rows: [MonacoInfoRow]
    let identifier: String

    @Environment(\.dismiss) private var dismiss
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var stacksRows: Bool { dynamicTypeSize >= .xxxLarge }

    private var rowLayout: AnyLayout {
        stacksRows
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs))
            : AnyLayout(HStackLayout(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            Text(message)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
            MonacoCard {
                VStack(spacing: 0) {
                    ForEach(Array(rows.enumerated()), id: \.offset) { index, row in
                        if index > 0 { Divider() }
                        rowLayout {
                            Text(row.label)
                                .font(MonacoTheme.Typo.headline)
                                .foregroundStyle(MonacoTheme.primaryText)
                                .fixedSize(horizontal: false, vertical: true)
                            if !stacksRows { Spacer(minLength: MonacoTheme.Space.s) }
                            Text(row.value)
                                .font(MonacoTheme.Typo.body)
                                .foregroundStyle(MonacoTheme.secondaryText)
                                .multilineTextAlignment(stacksRows ? .leading : .trailing)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        .padding(.vertical, MonacoTheme.Space.sm)
                        .accessibilityElement(children: .combine)
                    }
                }
            }
            Button(MarketStatsCopy.ok) { dismiss() }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("\(identifier)-ok")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoFittedSheet(title: title, titleIdentifier: "\(identifier)-title", showsClose: true)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(identifier)
    }
}
