#if DEBUG
import SwiftUI

struct GalleryPixelSamples: View {
    private var drawn: [MonacoIcon] { MonacoIcon.allCases.filter(\.hasPixel) }

    var body: some View {
        GalleryGroup(title: "Pixel glyphs") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Outline, 22") {
                    GalleryGrid(minimum: 44) {
                        ForEach(drawn, id: \.self) { icon in
                            MonacoGlyph(icon).foregroundStyle(MonacoTheme.ink)
                        }
                    }
                }
                GalleryCase(caption: "Filled, 22") {
                    GalleryGrid(minimum: 44) {
                        ForEach(drawn.filter { $0.pixel(filled: true) != $0.pixel() }, id: \.self) { icon in
                            MonacoGlyph(icon, filled: true).foregroundStyle(MonacoTheme.brand)
                        }
                    }
                }
                GalleryCase(caption: "Tab bar, 25, as the system draws them") {
                    HStack(spacing: MonacoTheme.Space.l) {
                        ForEach([MonacoIcon.home, .cabals, .markets, .feed, .profile], id: \.self) { icon in
                            VStack(spacing: MonacoTheme.Space.xs) {
                                MonacoGlyph(icon, size: MonacoGlyphMetrics.tab)
                                    .foregroundStyle(MonacoTheme.secondaryText)
                                MonacoGlyph(icon, filled: true, size: MonacoGlyphMetrics.tab)
                                    .foregroundStyle(MonacoTheme.brand)
                            }
                        }
                    }
                }
                GalleryCase(caption: "Small 16 and large 30") {
                    HStack(spacing: MonacoTheme.Space.m) {
                        MonacoGlyph(.chevron, size: MonacoGlyphMetrics.small)
                        MonacoGlyph(.check, size: MonacoGlyphMetrics.small)
                        MonacoGlyph(.close, size: MonacoGlyphMetrics.small)
                        MonacoGlyph(.checkCircle, size: MonacoGlyphMetrics.large)
                        MonacoGlyph(.alertCircle, size: MonacoGlyphMetrics.large)
                        MonacoGlyph(.offline, size: MonacoGlyphMetrics.large)
                    }
                    .foregroundStyle(MonacoTheme.ink)
                }
            }
        }
        GalleryGroup(title: "Cabal emblems") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "The cast, on each tint") {
                    GalleryGrid(minimum: 52) {
                        ForEach(Array(CabalEmblem.allCases.enumerated()), id: \.offset) { index, emblem in
                            let tint = MonacoTheme.CabalTint.allCases[index % MonacoTheme.CabalTint.allCases.count]
                            PixelGlyphShape(bitmap: emblem.bitmap)
                                .fill(tint.onFill, style: FillStyle(antialiased: false))
                                .frame(width: 32, height: 32)
                                .frame(width: 44, height: 44)
                                .background(
                                    tint.fill,
                                    in: RoundedRectangle(cornerRadius: MarkGeometry.radius(for: 44), style: .continuous)
                                )
                                .accessibilityLabel(emblem.spokenName)
                        }
                    }
                }
                GalleryCase(caption: "Pattern on a cover") {
                    CabalCover(cabalId: "gallery-pattern", tint: .plum, pictureURL: nil, style: .hero) {
                        Text("The Bingo Cabal")
                            .font(MonacoTheme.Typo.title)
                            .foregroundStyle(MonacoTheme.onCover)
                            .padding(MonacoTheme.Space.gutter)
                    }
                }
                GalleryCase(caption: "Marks, 44 and 24") {
                    HStack(spacing: MonacoTheme.Space.m) {
                        ForEach(GalleryFixtures.cabalNames.prefix(4), id: \.id) { cabal in
                            CabalMark(groupId: cabal.id, name: cabal.name, size: 44)
                            CabalMark(groupId: cabal.id, name: cabal.name, size: 24, shape: .circle)
                        }
                    }
                }
            }
        }
    }
}
#endif
