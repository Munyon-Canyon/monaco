import SwiftUI

protocol HomeSection: ScreenSection where Context == Void {}

struct HomeSectionHeader: View {
    let title: String
    var count: Int?
    var countText: String?
    let trailing: String
    var actionIdentifier: String?
    let action: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: 0) {
                header(trailing: nil)
                Button(action: action) {
                    HStack(spacing: MonacoTheme.Space.xs) {
                        Text(trailing)
                            .font(MonacoTheme.Typo.captionStrong)
                            .multilineTextAlignment(.leading)
                        Image(systemName: "chevron.right")
                            .font(MonacoTheme.Typo.captionStrong)
                            .accessibilityHidden(true)
                    }
                    .foregroundStyle(MonacoTheme.brand)
                    .frame(minHeight: SectionHeaderMetrics.hitHeight)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier(actionIdentifier ?? "")
            }
        } else {
            header(trailing: trailing)
        }
    }

    private var suffix: String? {
        if let countText { return countText }
        guard let count, count > 0 else { return nil }
        return "\(count)"
    }

    @ViewBuilder private func header(trailing: String?) -> some View {
        if let trailing, !dynamicTypeSize.isAccessibilitySize {
            HStack(spacing: MonacoTheme.Space.s) {
                titleLine
                Spacer(minLength: MonacoTheme.Space.s)
                Button(action: action) {
                    Text(trailing)
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.brand)
                        .lineLimit(1)
                        .frame(minHeight: SectionHeaderMetrics.hitHeight)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier(actionIdentifier ?? "")
            }
            .frame(minHeight: SectionHeaderMetrics.hitHeight)
            .padding(.bottom, SectionHeaderMetrics.bottomSpacing)
        } else {
            titleLine
                .frame(minHeight: SectionHeaderMetrics.hitHeight, alignment: .leading)
                .padding(.bottom, SectionHeaderMetrics.bottomSpacing)
        }
    }

    private var titleLine: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text(title)
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityAddTraits(.isHeader)
            if let suffix {
                Text(suffix)
                    .font(MonacoTheme.Typo.section.monospacedDigit())
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
    }
}
