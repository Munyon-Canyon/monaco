import SwiftUI

struct MonacoPeriodLabels<P: Hashable>: View {
    let title: String
    let dates: String
    let periods: [P]
    @Binding var selection: P
    let identifierPrefix: String
    let label: (P) -> String

    init(
        title: String, dates: String, periods: [P], selection: Binding<P>, identifierPrefix: String,
        label: @escaping (P) -> String
    ) {
        self.title = title
        self.dates = dates
        self.periods = periods
        _selection = selection
        self.identifierPrefix = identifierPrefix
        self.label = label
    }

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: MonacoTheme.Space.m) {
                heading
                Spacer(minLength: 0)
                labels
            }
            VStack(alignment: .leading, spacing: 0) {
                heading
                labels
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var heading: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text(title)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
            Text(dates)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(.isHeader)
    }

    private var labels: some View {
        MonacoPlainLabelRow(
            items: periods, selection: selection, spacing: MonacoPlainLabelMetrics.scrollSpacing,
            equalWidth: false, alwaysFits: true, scrollInset: 0, label: label, accessibilityName: label,
            identifier: { "\(identifierPrefix)-\(label($0))" }, onSelect: { selection = $0 })
    }
}
