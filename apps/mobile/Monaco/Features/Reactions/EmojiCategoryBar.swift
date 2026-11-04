import SwiftUI

struct EmojiCategory: Identifiable, Equatable {
    static let recent = "recent"
    static let smileys = "smileys"

    let id: String
    let icon: MonacoIcon
    let label: String

    static let all = [
        EmojiCategory(id: recent, icon: .clock, label: "Frequently used"),
        EmojiCategory(id: smileys, icon: .smiley, label: "Smileys and people"),
        EmojiCategory(id: "people", icon: .hand, label: "People and body"),
        EmojiCategory(id: "nature", icon: .leaf, label: "Animals and nature"),
        EmojiCategory(id: "flags", icon: .flag, label: "Flags"),
    ]
}

struct EmojiCategoryBar: View {
    let current: String
    let onSelect: (String) -> Void

    private static let minTap: CGFloat = 44

    @ScaledMetric(relativeTo: .title3) private var tapSize: CGFloat = 44

    var body: some View {
        VStack(spacing: 0) {
            Rectangle()
                .fill(MonacoTheme.border)
                .frame(height: 1)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            HStack(spacing: 0) {
                ForEach(EmojiCategory.all) { category in
                    Button {
                        Haptics.selection()
                        onSelect(category.id)
                    } label: {
                        Image(monaco: category.icon)
                            .font(.title3)
                            .foregroundStyle(
                                current == category.id ? MonacoTheme.primaryText : MonacoTheme.secondaryText
                            )
                            .frame(minWidth: min(tapSize, Self.minTap), minHeight: tapSize)
                            .modifier(SelectedCategoryPill(isSelected: current == category.id))
                            .frame(maxWidth: .infinity, minHeight: tapSize)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(category.label)
                    .accessibilityAddTraits(current == category.id ? .isSelected : [])
                    .accessibilityIdentifier("emoji-category-\(category.id)")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xs)
        }
    }
}

private struct SelectedCategoryPill: ViewModifier {
    let isSelected: Bool

    @MonacoReduceTransparency private var reduceTransparency
    @Environment(\.displayScale) private var displayScale

    @ViewBuilder
    func body(content: Content) -> some View {
        if !isSelected {
            content
        } else if reduceTransparency {
            content
                .background(Circle().fill(MonacoTheme.surface))
                .overlay(Circle().strokeBorder(MonacoTheme.hairline, lineWidth: 1 / displayScale))
        } else if #available(iOS 26, *) {
            content.glassEffect(.regular.interactive(), in: .circle)
        } else {
            content.background(Circle().fill(MonacoTheme.border))
        }
    }
}
