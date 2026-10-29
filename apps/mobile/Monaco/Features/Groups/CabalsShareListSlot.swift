import MonacoCore
import SwiftUI

enum CabalsShareListSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        CabalsShareListSection()
    }
}

struct CabalsShareListSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(CabalsTabStore.self) private var store

    var body: some View {
        let tiles = store.tiles
        VStack(alignment: .leading, spacing: 0) {
            if !tiles.isEmpty {
                MonacoSectionHeader("Your share in each")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.l)
                ForEach(tiles) { tile in
                    row(tile, isLast: tile.id == tiles.last?.id)
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-share-list")
    }

    private func row(_ tile: CabalTile, isLast: Bool) -> some View {
        Button {
            Haptics.selection()
            environment.navigator.open(CabalRoute(id: tile.id), in: .cabals)
        } label: {
            MonacoRow(title: tile.shortName, subtitle: tile.shareLine, isLast: isLast) {
                CabalMark(groupId: tile.id, name: tile.name, size: .row, pictureUrl: tile.pictureURL)
            } trailing: {
                VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                    Text(tile.valueText)
                        .font(MonacoTheme.Typo.headline.monospacedDigit())
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(tile.returnText)
                        .font(MonacoTheme.Typo.caption.monospacedDigit())
                        .foregroundStyle(tile.isFunded ? tile.direction.pnlTone.color : MonacoTheme.secondaryText)
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("cabals-share-row-\(tile.id)")
    }
}
