#if DEBUG
import SwiftUI

struct GalleryCoverSamples: View {
    private let styles: [(name: String, style: CabalCover<AnyView>.Style, height: CGFloat?, minimum: CGFloat)] = [
        ("hero", .hero, nil, 104), ("heroTall", .heroTall, nil, 104), ("card", .card, 110, 150),
        ("memberCard", .memberCard, 124, 150),
    ]

    var body: some View {
        GalleryGroup(title: "Covers") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                ForEach(styles, id: \.name) { entry in
                    GalleryCase(caption: "\(entry.name): five tints, without a picture, then with one") {
                        GalleryGrid(minimum: entry.minimum) {
                            ForEach([false, true], id: \.self) { withPicture in
                                ForEach(MonacoTheme.CabalTint.allCases, id: \.self) { tint in
                                    cover(entry.style, height: entry.height, tint: tint, withPicture: withPicture)
                                }
                            }
                        }
                    }
                }
                GalleryCase(caption: "thumb 44: five tints, without a picture, then with one") {
                    GalleryGrid(minimum: 52) {
                        ForEach([false, true], id: \.self) { withPicture in
                            ForEach(MonacoTheme.CabalTint.allCases, id: \.self) { tint in
                                cover(.thumb(44), height: 44, tint: tint, withPicture: withPicture)
                                    .frame(width: 44)
                            }
                        }
                    }
                }
                GalleryCase(caption: "CoverShareDisc on a hero cover") {
                    CabalCover(cabalId: "gallery-share", tint: .purple, pictureURL: nil, style: .hero) {
                        CoverShareDisc(cabalName: "Weekend investors") {}
                            .padding(MonacoTheme.Space.s)
                    }
                }
            }
        }
    }

    private func cover(
        _ style: CabalCover<AnyView>.Style, height: CGFloat?, tint: MonacoTheme.CabalTint, withPicture: Bool
    ) -> some View {
        CabalCover(
            cabalId: "gallery-\(tint.galleryName)", tint: tint,
            pictureURL: withPicture ? GalleryFixtures.picture(tint, name: "gallery-cover-\(tint.galleryName)") : nil,
            style: style
        ) {
            AnyView(
                Text(tint.galleryName)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.onCover)
                    .padding(MonacoTheme.Space.s))
        }
        .frame(height: height)
    }
}

struct GalleryWordmarkSamples: View {
    var body: some View {
        GalleryGroup(title: "Wordmark") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                GalleryCase(caption: "Toolbar") { MonacoWordmark(.toolbar) }
                GalleryCase(caption: "Sign-in") { MonacoWordmark(.signIn) }
            }
        }
    }
}

struct GalleryIconSamples: View {
    var body: some View {
        GalleryGroup(title: "Icons") {
            GalleryGrid(minimum: 150) {
                ForEach(MonacoIcon.allCases, id: \.self) { icon in
                    HStack(spacing: MonacoTheme.Space.sm) {
                        Image(monaco: icon)
                            .font(.title3)
                            .foregroundStyle(MonacoTheme.ink)
                            .frame(width: 32)
                        GalleryCaption(String(describing: icon))
                    }
                    .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
                    .accessibilityElement(children: .combine)
                }
            }
        }
    }
}
#endif
