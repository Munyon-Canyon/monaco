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
                            .frame(minWidth: tapSize, minHeight: tapSize)
                            .background {
                                if current == category.id { Circle().fill(MonacoTheme.border) }
                            }
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
