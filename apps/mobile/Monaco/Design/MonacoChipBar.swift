import SwiftUI

struct MonacoChipBar<Item: Hashable>: View {
    let items: [Item]
    let selected: Item
    let title: (Item) -> String
    let identifierPrefix: String
    let select: (Item) -> Void

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: MonacoTheme.Space.s) {
                ForEach(items, id: \.self) { item in
                    let isSelected = item == selected
                    Button {
                        guard !isSelected else { return }
                        Haptics.selection()
                        select(item)
                    } label: {
                        Text(title(item))
                            .font(MonacoTheme.Typo.calloutStrong)
                            .foregroundStyle(isSelected ? MonacoTheme.primaryButtonLabel : MonacoTheme.ink)
                            .padding(.horizontal, MonacoTheme.Space.m)
                            .frame(minHeight: 36)
                            .background(
                                isSelected ? MonacoTheme.primaryButtonFill : MonacoTheme.surfaceSunken, in: Capsule()
                            )
                            .frame(minHeight: 44)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(isSelected ? [.isSelected] : [])
                    .accessibilityIdentifier("\(identifierPrefix)-\(title(item))")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.m)
        }
    }
}
