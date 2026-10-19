#if DEBUG
import SwiftUI

struct GalleryMarksPage: View {
    var body: some View {
        GalleryPage(title: "Marks") {
            GalleryGlyphSamples()
            GalleryStockSamples()
            GalleryCabalMarkSamples()
            GalleryAvatarSamples()
            GalleryIdentitySamples()
            GalleryCoverSamples()
            GalleryWordmarkSamples()
            GalleryIconSamples()
        }
    }
}

struct GalleryGlyphSamples: View {
    var body: some View {
        GalleryGroup(title: "Glyph tiles") {
            HStack(alignment: .top, spacing: MonacoTheme.Space.l) {
                GalleryCase(caption: "Normal") { SunkenGlyphMark(icon: .deposit) }
                GalleryCase(caption: "Muted") { SunkenGlyphMark(icon: .deposit, isMuted: true) }
            }
        }
    }
}

struct GalleryStockSamples: View {
    private let sizes: [(name: String, size: StockMarkSize)] = [
        ("feed 28", .feed), ("mover 31", .mover), ("card 35", .card), ("row 42", .row), ("header 47", .header),
        ("review 48", .review), ("receipt 54", .receipt),
    ]

    private let logo = GalleryFixtures.picture(.indigo, name: "gallery-logo")

    var body: some View {
        GalleryGroup(title: "Stock marks") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Ticker fallback, seven sizes, and cash") {
                    GalleryGrid(minimum: 84) {
                        ForEach(sizes, id: \.name) { entry in
                            GalleryCase(caption: entry.name) { StockMark(symbol: "NVDAx", size: entry.size) }
                        }
                        GalleryCase(caption: "cash") { StockMark(symbol: "USDC", size: .row) }
                    }
                }
                GalleryCase(caption: "Issuer logo, seven sizes") {
                    GalleryGrid(minimum: 84) {
                        ForEach(sizes, id: \.name) { entry in
                            GalleryCase(caption: entry.name) {
                                StockMark(symbol: "NVDAx", size: entry.size, logoURL: logo)
                            }
                        }
                    }
                }
            }
        }
    }
}

struct GalleryCabalMarkSamples: View {
    var body: some View {
        GalleryGroup(title: "Cabal marks") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Tile") {
                    GalleryGrid(minimum: 72) {
                        ForEach(GalleryFixtures.cabalNames, id: \.id) { cabal in
                            CabalMark(groupId: cabal.id, name: cabal.name, size: 44)
                        }
                    }
                }
                GalleryCase(caption: "Circle") {
                    GalleryGrid(minimum: 72) {
                        ForEach(GalleryFixtures.cabalNames, id: \.id) { cabal in
                            CabalMark(groupId: cabal.id, name: cabal.name, size: 44, shape: .circle)
                        }
                    }
                }
            }
        }
    }
}
#endif
