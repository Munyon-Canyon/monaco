import SwiftUI

struct MonacoChipBar<Item: Hashable>: View {
    let items: [Item]
    let selected: Item
    let title: (Item) -> String
    let identifierPrefix: String
    var maxVisible = 4
    let select: (Item) -> Void

    static func fits(_ count: Int, maxVisible: Int) -> Bool { count <= maxVisible }

    var body: some View {
        let _ = assert(Self.fits(items.count, maxVisible: maxVisible), "A chip bar shows at most \(maxVisible) chips")
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
