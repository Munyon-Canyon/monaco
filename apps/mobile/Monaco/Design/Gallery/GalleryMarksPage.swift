#if DEBUG
import SwiftUI

struct GalleryMarksPage: View {
    var body: some View {
        GalleryPage(title: "Marks") {
            GalleryGlyphSamples()
            GalleryStockSamples()
            GalleryCabalMarkSamples()
            GalleryCrestSamples()
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

struct GalleryCrestSamples: View {
    private let picture = GalleryFixtures.picture(.teal, name: "gallery-crest")?.absoluteString
    private let cabal = GalleryFixtures.cabalNames[0]

    var body: some View {
        GalleryGroup(title: "Crests") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Identity hue, 24, 32, 40 and 56, beside a person circle") {
                    HStack(alignment: .bottom, spacing: MonacoTheme.Space.sm) {
                        ForEach(CabalMark.Size.allCases, id: \.rawValue) { size in
                            CabalMark(groupId: cabal.id, name: cabal.name, size: size)
                        }
                        MonacoAvatar(photoURL: nil, displayName: "Andres", size: 40, seed: "gallery-person")
                    }
                }
                GalleryCase(caption: "With a picture, 24, 32, 40 and 56") {
                    HStack(alignment: .bottom, spacing: MonacoTheme.Space.sm) {
                        ForEach(CabalMark.Size.allCases, id: \.rawValue) { size in
                            CabalMark(groupId: cabal.id, name: cabal.name, size: size, pictureUrl: picture)
                        }
                    }
                }
                GalleryCase(caption: "Vote ring and badge, 1 and 12") {
                    HStack(spacing: MonacoTheme.Space.l) {
                        CabalMark(groupId: cabal.id, name: cabal.name, size: .row)
                            .crestVoteRing(true)
                            .crestBadge(1)
                        CabalMark(groupId: GalleryFixtures.cabalNames[1].id, name: "Semis or bust", size: .row)
                            .crestVoteRing(true)
                            .crestBadge(12)
                    }
                    .padding(MonacoTheme.Space.s)
                }
                GalleryCase(caption: "A 24 crest on a photo's corner") {
                    HStack(spacing: MonacoTheme.Space.m) {
                        MonacoAvatar(photoURL: nil, displayName: "Rosa", size: 40, seed: "gallery-rosa")
                            .crestOnCorner(groupId: cabal.id, name: cabal.name)
                        MonacoAvatar(photoURL: nil, displayName: "Kai", size: 40, seed: "gallery-kai")
                            .crestOnCorner(groupId: cabal.id, name: cabal.name, pictureUrl: picture)
                    }
                }
            }
        }
    }
}
#endif
