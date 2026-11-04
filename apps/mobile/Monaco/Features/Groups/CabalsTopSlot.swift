import MonacoCore
import SwiftUI

enum CabalsTopSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        CabalsTopSection()
    }
}

enum CabalsTopMetrics {
    static let tileWidth: CGFloat = 148
    static let coverHeight: CGFloat = 96
}

struct CabalsTopSection: View {
    @Environment(CabalsTabStore.self) private var store
    @Environment(AppEnvironment.self) private var environment

    private var tiles: [CabalsTopTile] {
        guard store.content?.showsTopCabals == true, case .ready(let tiles) = store.top.phase else { return [] }
        return tiles
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if !tiles.isEmpty {
                MonacoSectionHeader(
                    "Top cabals this month", trailing: "Leaderboard", actionIdentifier: "cabals-top-leaderboard"
                ) {
                    environment.navigator.open(LeaderboardRoute(tab: .cabals, period: .month), in: .cabals)
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                ScrollView(.horizontal, showsIndicators: false) {
                    LazyHStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                        ForEach(tiles) { tile in
                            CabalsTopTileView(tile: tile)
                        }
                    }
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                }
                .padding(.bottom, MonacoTheme.Space.m)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-top")
    }
}

private struct CabalsTopTileView: View {
    let tile: CabalsTopTile

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        Button {
            Haptics.selection()
            environment.navigator.open(CabalRoute(id: tile.id), in: .cabals)
        } label: {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                CabalCover(
                    cabalId: tile.id, tint: .forGroupId(tile.id), pictureURL: CabalMark.resolvedURL(tile.pictureURL),
                    style: .art
                )
                .frame(width: CabalsTopMetrics.tileWidth, height: CabalsTopMetrics.coverHeight)
                Text(tile.name)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                Text(tile.line)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
                    .multilineTextAlignment(.leading)
            }
            .frame(width: CabalsTopMetrics.tileWidth, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(tile.name), \(tile.line.replacingOccurrences(of: " · ", with: ", "))")
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("cabals-top-tile-\(tile.id)")
    }
}
