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

    @ViewBuilder private func header(trailing: String?) -> some View {
        let base = MonacoSectionHeader(
            title, count: count, trailing: trailing, actionIdentifier: actionIdentifier, action: action)
        if let countText { base.countText(countText) } else { base }
    }
}
