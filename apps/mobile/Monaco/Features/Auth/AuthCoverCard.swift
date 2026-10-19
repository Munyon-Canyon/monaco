import SwiftUI

struct AuthCoverCard: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private static let title = "One pot. Shared decisions."
    private static let message = "Join a cabal with people you know, or start your own."
    private static let tile: CGFloat = 98
    private static let compactTile: CGFloat = 64
    private static let markFraction: CGFloat = 44 / 98
    private static let minHeight: CGFloat = 132

    private var isStacked: Bool { dynamicTypeSize.isAccessibilitySize }

    private var tileSize: CGFloat { isStacked ? Self.compactTile : Self.tile }

    private var layout: AnyLayout {
        isStacked
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.sm))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
    }

    var body: some View {
        MonacoCard {
            layout {
                cover
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(Self.title)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.ink)
                    Text(Self.message)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                }
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .frame(minHeight: Self.minHeight - 2 * MonacoTheme.Space.m)
        }
        .padding(.vertical, MonacoTheme.Space.xl)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(Self.title) \(Self.message)")
    }

    private var cover: some View {
        CabalCover(cabalId: "auth", tint: .pine, pictureURL: nil, style: .thumb(tileSize))
            .overlay {
                MonacoMark(size: tileSize * Self.markFraction)
                    .environment(\.colorScheme, .dark)
            }
            .accessibilityHidden(true)
    }
}

#Preview {
    AuthCoverCard()
        .padding(.horizontal, MonacoTheme.Space.gutter)
}
