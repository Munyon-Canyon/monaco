#if DEBUG
import SwiftUI

struct GalleryToken: Identifiable {
    let name: String
    let color: Color
    var id: String { name }
}

struct GalleryTokenGroup: Identifiable {
    let title: String
    let tokens: [GalleryToken]
    var id: String { title }
}

enum GalleryTokenCatalog {
    typealias Theme = MonacoTheme

    static let groups: [GalleryTokenGroup] = [
        GalleryTokenGroup(
            title: "Surfaces",
            tokens: [
                GalleryToken(name: "canvas", color: Theme.canvas),
                GalleryToken(name: "surface", color: Theme.surface),
                GalleryToken(name: "surfaceSunken", color: Theme.surfaceSunken),
                GalleryToken(name: "hairline", color: Theme.hairline),
                GalleryToken(name: "scrim", color: Theme.scrim),
            ]),
        GalleryTokenGroup(
            title: "Text",
            tokens: [
                GalleryToken(name: "ink", color: Theme.ink),
                GalleryToken(name: "bodyText", color: Theme.bodyText),
                GalleryToken(name: "muted", color: Theme.muted),
                GalleryToken(name: "tertiaryText", color: Theme.tertiaryText),
                GalleryToken(name: "disabledLabel", color: Theme.disabledLabel),
            ]),
        GalleryTokenGroup(
            title: "Brand",
            tokens: [
                GalleryToken(name: "accent", color: Theme.accent),
                GalleryToken(name: "brandFill", color: Theme.brandFill),
                GalleryToken(name: "onBrand", color: Theme.onBrand),
                GalleryToken(name: "brandSoft", color: Theme.brandSoft),
                GalleryToken(name: "disabled", color: Theme.disabled),
            ]),
        GalleryTokenGroup(
            title: "Money",
            tokens: [
                GalleryToken(name: "profit", color: Theme.profit),
                GalleryToken(name: "loss", color: Theme.loss),
                GalleryToken(name: "profitVivid", color: Theme.profitVivid),
                GalleryToken(name: "lossVivid", color: Theme.lossVivid),
                GalleryToken(name: "warning", color: Theme.warning),
            ]),
        GalleryTokenGroup(
            title: "Washes",
            tokens: [
                GalleryToken(name: "warningWash", color: Theme.warningWash),
                GalleryToken(name: "buyWash", color: Theme.buyWash),
                GalleryToken(name: "sellWash", color: Theme.sellWash),
                GalleryToken(name: "priceWash", color: Theme.priceWash),
            ]),
        GalleryTokenGroup(
            title: "Glass",
            tokens: [
                GalleryToken(name: "glassFill", color: Theme.glassFill),
                GalleryToken(name: "glassStroke", color: Theme.glassStroke),
            ]),
        GalleryTokenGroup(
            title: "Sheet",
            tokens: [
                GalleryToken(name: "sheetGlass", color: Theme.sheetGlass),
                GalleryToken(name: "sheetStroke", color: Theme.sheetStroke),
                GalleryToken(name: "sheetDim", color: Theme.sheetDim),
            ]),
        GalleryTokenGroup(
            title: "Toast",
            tokens: [
                GalleryToken(name: "toastFill", color: Theme.toastFill),
                GalleryToken(name: "toastLabel", color: Theme.toastLabel),
                GalleryToken(name: "toastSuccessGlyph", color: Theme.toastSuccessGlyph),
                GalleryToken(name: "toastErrorGlyph", color: Theme.toastErrorGlyph),
            ]),
        GalleryTokenGroup(
            title: "Identity card",
            tokens: [
                GalleryToken(name: "identityFill", color: Theme.identityFill),
                GalleryToken(name: "onIdentity", color: Theme.onIdentity),
                GalleryToken(name: "onIdentityMuted", color: Theme.onIdentityMuted),
                GalleryToken(name: "onIdentityBody", color: Theme.onIdentityBody),
                GalleryToken(name: "profitOnIdentity", color: Theme.profitOnIdentity),
                GalleryToken(name: "lossOnIdentity", color: Theme.lossOnIdentity),
                GalleryToken(name: "pencilFill", color: Theme.pencilFill),
            ]),
        GalleryTokenGroup(
            title: "Cover",
            tokens: [
                GalleryToken(name: "onCover", color: Theme.onCover),
                GalleryToken(name: "onCoverMuted", color: Theme.onCoverMuted),
                GalleryToken(name: "profitOnCover", color: Theme.profitOnCover),
                GalleryToken(name: "lossOnCover", color: Theme.lossOnCover),
                GalleryToken(name: "onCoverGlass", color: Theme.onCoverGlass),
                GalleryToken(name: "onCoverGlassStroke", color: Theme.onCoverGlassStroke),
                GalleryToken(name: "coverStroke", color: Theme.coverStroke),
            ]),
        GalleryTokenGroup(
            title: "Vote needed and discussion",
            tokens: [
                GalleryToken(name: "voteNeededFill", color: Theme.voteNeededFill),
                GalleryToken(name: "voteNeededLabel", color: Theme.voteNeededLabel),
                GalleryToken(name: "discussionTint", color: Theme.discussionTint),
                GalleryToken(name: "discussionWash", color: Theme.discussionWash),
            ]),
        GalleryTokenGroup(
            title: "Allocation tints",
            tokens: Theme.allocationTints.enumerated().map {
                GalleryToken(name: "allocationTints[\($0.offset)]", color: $0.element)
            }),
        GalleryTokenGroup(
            title: "Cabal tints",
            tokens: Theme.CabalTint.allCases.map { GalleryToken(name: "CabalTint.\($0.galleryName)", color: $0.fill) }
        ),
        GalleryTokenGroup(
            title: "Gold",
            tokens: [
                GalleryToken(name: "gold", color: Theme.gold),
                GalleryToken(name: "goldGlyph", color: Theme.goldGlyph),
                GalleryToken(name: "goldWash", color: Theme.goldWash),
            ]),
    ]
}

struct GallerySwatch: View {
    let color: Color
    let scheme: ColorScheme

    var body: some View {
        RoundedRectangle(cornerRadius: MonacoTheme.Radius.badge, style: .continuous)
            .fill(color)
            .frame(width: 44, height: 44)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.badge, style: .continuous)
                    .fill(MonacoTheme.canvas)
            )
            .overlay(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.badge, style: .continuous)
                    .strokeBorder(MonacoTheme.hairline, lineWidth: 1)
            )
            .environment(\.colorScheme, scheme)
            .accessibilityHidden(true)
    }
}

struct GalleryTokenRow: View {
    let token: GalleryToken

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var layout: AnyLayout {
        dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
    }

    private var hexLine: String {
        "light \(GalleryHex.string(token.color, in: .light))  dark \(GalleryHex.string(token.color, in: .dark))"
    }

    var body: some View {
        layout {
            HStack(spacing: MonacoTheme.Space.sm) {
                GallerySwatch(color: token.color, scheme: .light)
                GallerySwatch(color: token.color, scheme: .dark)
            }
            VStack(alignment: .leading, spacing: 0) {
                Text(token.name)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.ink)
                Text(hexLine)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
            .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .frame(minHeight: 44)
        .accessibilityElement(children: .combine)
    }
}

struct GalleryScrimSamples: View {
    var body: some View {
        GalleryGroup(title: "Cover overlay over a tint") {
            GalleryGrid(minimum: 150) {
                ForEach(Array(MonacoTheme.CabalTint.allCases.prefix(2).enumerated()), id: \.offset) { index, tint in
                    GalleryCase(caption: index == 0 ? "coverOverlay" : "coverOverlayReduced") {
                        ZStack {
                            tint.fill
                            index == 0 ? MonacoTheme.coverOverlay : MonacoTheme.coverOverlayReduced
                        }
                        .frame(height: 112)
                        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.cardTight, style: .continuous))
                    }
                }
            }
        }
    }
}

struct GalleryMetricSamples: View {
    private let spacing: [(name: String, value: CGFloat)] = [
        ("xs", MonacoTheme.Space.xs), ("s", MonacoTheme.Space.s), ("sm", MonacoTheme.Space.sm),
        ("m", MonacoTheme.Space.m), ("gutter", MonacoTheme.Space.gutter), ("l", MonacoTheme.Space.l),
        ("xl", MonacoTheme.Space.xl),
    ]

    private let radii: [(name: String, value: CGFloat)] = [
        ("card", MonacoTheme.Radius.card), ("cardTight", MonacoTheme.Radius.cardTight),
        ("field", MonacoTheme.Radius.field), ("search", MonacoTheme.Radius.search),
        ("notice", MonacoTheme.Radius.notice), ("toast", MonacoTheme.Radius.toast),
        ("badge", MonacoTheme.Radius.badge), ("pill", MonacoTheme.Radius.pill),
        ("identity", MonacoTheme.Radius.identity),
    ]

    var body: some View {
        GalleryGroup(title: "Shadow") {
            let layer = MonacoTheme.Shadow.floating
            GalleryCase(caption: "Shadow.floating") {
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                    .fill(MonacoTheme.surface)
                    .frame(height: 72)
                    .shadow(color: layer.color, radius: layer.radius, x: 0, y: layer.y)
                    .padding(.bottom, MonacoTheme.Space.s)
            }
        }
        GalleryGroup(title: "Spacing") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                ForEach(spacing, id: \.name) { entry in
                    HStack(spacing: MonacoTheme.Space.sm) {
                        RoundedRectangle(cornerRadius: MonacoTheme.Radius.pill, style: .continuous)
                            .fill(MonacoTheme.brandFill)
                            .frame(width: entry.value, height: 16)
                        GalleryCaption("Space.\(entry.name) \(Int(entry.value))")
                    }
                }
            }
        }
        GalleryGroup(title: "Radius") {
            GalleryGrid(minimum: 96) {
                ForEach(radii, id: \.name) { entry in
                    GalleryCase(caption: "\(entry.name) \(Int(entry.value))") {
                        RoundedRectangle(cornerRadius: entry.value, style: .continuous)
                            .fill(MonacoTheme.surface)
                            .overlay(
                                RoundedRectangle(cornerRadius: entry.value, style: .continuous)
                                    .strokeBorder(MonacoTheme.hairline, lineWidth: 1)
                            )
                            .frame(height: 56)
                    }
                }
            }
        }
    }
}

struct GalleryTokensPage: View {
    var body: some View {
        GalleryPage(title: "Tokens") {
            ForEach(GalleryTokenCatalog.groups) { group in
                GalleryGroup(title: group.title) {
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                        ForEach(group.tokens) { GalleryTokenRow(token: $0) }
                    }
                }
            }
            GalleryScrimSamples()
            GalleryMetricSamples()
        }
    }
}
#endif
