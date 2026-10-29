import MonacoCore
import SwiftUI

extension EnvironmentValues {
    @Entry var frequentEmojiStore: FrequentEmojiStore?
}

extension EmojiCatalog {
    static let bundled: EmojiCatalog = {
        guard let url = Bundle.main.url(forResource: "emoji", withExtension: "json"),
            let data = try? Data(contentsOf: url),
            let catalog = try? EmojiCatalog.load(from: data)
        else { return EmojiCatalog(entries: []) }
        return catalog
    }()
}

private struct EmojiSection: Identifiable {
    let id: String
    let title: String
    let emojis: [String]
    let identifierPrefix: String
}

private enum PickerMetrics {
    static let sheetHeight: CGFloat = 426
    static let columns = 8
    static let accessibilityColumns = 4
    static let cell: CGFloat = 44
    static let headerHeight: CGFloat = 32
    static let sectionGap: CGFloat = 8
    static let gridInset = MonacoTheme.Space.xs
    static let headerInset = MonacoTheme.Space.gutter - gridInset

    static func height(of section: EmojiSection, columns: Int, cell: CGFloat, headerHeight: CGFloat) -> CGFloat {
        let rows = (section.emojis.count + columns - 1) / columns
        return headerHeight + CGFloat(rows) * cell + sectionGap
    }
}

struct EmojiPickerSheet: View {
    let onPick: (String) -> Void
    let catalog: EmojiCatalog

    @Environment(\.dismiss) private var dismiss
    @Environment(\.frequentEmojiStore) private var injectedStore
    @State private var standardStore = FrequentEmojiStore(defaults: .standard)

    @State private var query: String
    @State private var current = EmojiCategory.smileys
    @State private var recent: [String] = []
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .title) private var scaledCell = PickerMetrics.cell
    @ScaledMetric(relativeTo: .subheadline) private var scaledHeader = PickerMetrics.headerHeight

    private var isLarge: Bool { dynamicTypeSize.isAccessibilitySize }

    private var columns: Int { isLarge ? PickerMetrics.accessibilityColumns : PickerMetrics.columns }

    private var cell: CGFloat { isLarge ? scaledCell : PickerMetrics.cell }

    private var headerHeight: CGFloat { isLarge ? scaledHeader : PickerMetrics.headerHeight }

    init(initialQuery: String = "", catalog: EmojiCatalog = .bundled, onPick: @escaping (String) -> Void) {
        _query = State(initialValue: initialQuery)
        self.catalog = catalog
        self.onPick = onPick
    }

    private static let titles: [EmojiGroup: String] = [
        .smileys: "Smileys and people", .people: "People and body", .nature: "Animals and nature",
        .food: "Food and drink", .activities: "Activities", .travel: "Travel and places",
        .objects: "Objects", .symbols: "Symbols", .flags: "Flags",
    ]

    private var sections: [EmojiSection] {
        let groups = EmojiGroup.allCases.map { group in
            EmojiSection(
                id: group.rawValue, title: Self.titles[group] ?? group.rawValue,
                emojis: catalog.entries(in: group).map(\.emoji), identifierPrefix: "emoji-cell-")
        }
        let frequentSection = EmojiSection(
            id: EmojiCategory.recent, title: "Frequently used", emojis: recent,
            identifierPrefix: "emoji-frequent-")
        return [frequentSection] + groups
    }

    private var frequent: FrequentEmojiStore { injectedStore ?? standardStore }

    private var trimmedQuery: String { query.trimmingCharacters(in: .whitespaces) }

    var body: some View {
        VStack(spacing: 0) {
            searchField
            if trimmedQuery.isEmpty {
                browse
            } else {
                results
            }
        }
        .padding(.top, MonacoTheme.Space.l)
        .presentationDetents(isLarge ? [.large] : [.height(PickerMetrics.sheetHeight)])
        .presentationDragIndicator(.visible)
        .presentationBackground(MonacoTheme.canvas)
        .presentationContentInteraction(.scrolls)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("emoji-picker")
        .onAppear { recent = frequent.emojis }
    }

    private var searchField: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Image(monaco: .search).foregroundStyle(MonacoTheme.secondaryText)
            TextField("Search emoji", text: $query)
                .font(.body)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .submitLabel(.search)
                .accessibilityIdentifier("emoji-search")
            if !query.isEmpty {
                Button {
                    query = ""
                } label: {
                    Image(monaco: .close)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .frame(width: cell, height: cell)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Clear search")
                .accessibilityIdentifier("emoji-search-clear")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.sm)
        .frame(minHeight: cell)
        .background(Capsule().fill(MonacoTheme.border.opacity(0.5)))
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.xs)
    }

    private var browse: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0) {
                    ForEach(sections) { section in
                        sectionView(section).id(section.id)
                    }
                }
                .padding(.horizontal, PickerMetrics.gridInset)
            }
            .onScrollGeometryChange(for: CGFloat.self) {
                $0.contentOffset.y + $0.contentInsets.top
            } action: { _, offset in
                current = section(at: offset)
            }
            EmojiCategoryBar(current: current) { id in
                current = id
                withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo(id, anchor: .top) }
            }
        }
    }

    private func section(at offset: CGFloat) -> String {
        var top: CGFloat = 0
        var found = EmojiCategory.smileys
        for section in sections {
            let isTab = section.id != EmojiCategory.recent && EmojiCategory.all.contains(where: { $0.id == section.id })
            if top <= offset + 1, isTab { found = section.id }
            top += PickerMetrics.height(of: section, columns: columns, cell: cell, headerHeight: headerHeight)
        }
        return found
    }

    private var results: some View {
        let found = catalog.search(trimmedQuery)
        return ScrollView {
            if found.isEmpty {
                Text("No emoji found")
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .frame(maxWidth: .infinity, minHeight: 120)
                    .accessibilityIdentifier("emoji-search-empty")
            } else {
                grid(found.map(\.emoji), identifierPrefix: "emoji-cell-")
                    .padding(.horizontal, PickerMetrics.gridInset)
            }
        }
    }

    private func sectionView(_ section: EmojiSection) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(section.title)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: isLarge)
                .frame(
                    maxWidth: .infinity, minHeight: headerHeight, maxHeight: isLarge ? nil : headerHeight,
                    alignment: .leading
                )
                .padding(.horizontal, PickerMetrics.headerInset)
                .accessibilityAddTraits(.isHeader)
            grid(section.emojis, identifierPrefix: section.identifierPrefix)
            Color.clear.frame(height: PickerMetrics.sectionGap)
        }
    }

    private func grid(_ emojis: [String], identifierPrefix: String) -> some View {
        LazyVGrid(
            columns: Array(repeating: GridItem(.flexible(), spacing: 0), count: columns),
            spacing: 0
        ) {
            ForEach(emojis, id: \.self) { emoji in
                Button {
                    pick(emoji)
                } label: {
                    Text(emoji)
                        .font(.title)
                        .frame(maxWidth: .infinity)
                        .frame(minHeight: cell)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(catalog.name(for: emoji) ?? emoji.reactionName)
                .accessibilityIdentifier("\(identifierPrefix)\(emoji.reactionHex)")
            }
        }
    }

    private func pick(_ emoji: String) {
        Haptics.selection()
        frequent.record(emoji)
        onPick(emoji)
        dismiss()
    }
}
