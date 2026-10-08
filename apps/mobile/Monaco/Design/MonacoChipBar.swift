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
                        Text(title(item)).monacoChipLabel(isSelected: isSelected)
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(isSelected ? [.isSelected] : [])
                    .accessibilityIdentifier("\(identifierPrefix)-\(title(item))")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }
}
