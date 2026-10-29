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
            title: "Identity hues",
            tokens: Theme.CabalTint.allCases.map { GalleryToken(name: "CabalTint.\($0.galleryName)", color: $0.fill) }
        ),
        GalleryTokenGroup(
            title: "Badge",
            tokens: [
                GalleryToken(name: "voteNeededFill", color: Theme.voteNeededFill),
                GalleryToken(name: "voteNeededLabel", color: Theme.voteNeededLabel),
            ]),
        GalleryTokenGroup(
            title: "Other roles",
            tokens: [
                GalleryToken(name: "onBrand", color: Theme.onBrand),
                GalleryToken(name: "scrim", color: Theme.scrim),
                GalleryToken(name: "disabledLabel", color: Theme.disabledLabel),
                GalleryToken(name: "disabled", color: Theme.disabled),
                GalleryToken(name: "profitVivid", color: Theme.profitVivid),
                GalleryToken(name: "lossVivid", color: Theme.lossVivid),
                GalleryToken(name: "onDestructive", color: Theme.onDestructive),
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
            title: "Discussion",
            tokens: [
                GalleryToken(name: "discussionTint", color: Theme.discussionTint),
                GalleryToken(name: "discussionWash", color: Theme.discussionWash),
            ]),
        GalleryTokenGroup(
            title: "Allocation tints",
            tokens: Theme.allocationTints.enumerated().map {
                GalleryToken(name: "allocationTints[\($0.offset)]", color: $0.element)
            }),
    ]
}

struct GalleryColourRole: Identifiable {
    let name: String
    let color: Color
    let dark: String
    let light: String
    var id: String { name }

    static let board: [GalleryColourRole] = [
        GalleryColourRole(name: "canvas", color: MonacoTheme.canvas, dark: "", light: ""),
        GalleryColourRole(name: "cell", color: MonacoTheme.surface, dark: "", light: ""),
        GalleryColourRole(name: "fill", color: MonacoTheme.surfaceSunken, dark: "", light: ""),
        GalleryColourRole(name: "separator", color: MonacoTheme.hairline, dark: "", light: ""),
        GalleryColourRole(
            name: "label", color: MonacoTheme.primaryText,
            dark: "21.0:1 · 17.0:1 on cell", light: "21.0:1 · 18.8:1 on cell"),
        GalleryColourRole(
            name: "secondary", color: MonacoTheme.secondaryText,
            dark: "8.2:1 · 6.6:1 on cell", light: "5.2:1 · 4.7:1 on cell"),
        GalleryColourRole(name: "tertiary", color: MonacoTheme.tertiaryText, dark: "glyphs only", light: "glyphs only"),
        GalleryColourRole(
            name: "tint", color: MonacoTheme.tint,
            dark: "7.6:1 · 6.2:1 on cell", light: "7.9:1 · 7.1:1 on cell"),
        GalleryColourRole(
            name: "brand", color: MonacoTheme.brandFill,
            dark: "white text 7.9:1", light: "white text 7.9:1"),
        GalleryColourRole(
            name: "gain", color: MonacoTheme.profit,
            dark: "14.5:1 · 11.7:1 on cell", light: "4.8:1 · 4.3:1 on cell"),
        GalleryColourRole(
            name: "loss", color: MonacoTheme.lossVivid,
            dark: "6.2:1 · 5.0:1 on cell", light: "5.4:1 · 4.8:1 on cell"),
        GalleryColourRole(name: "crown", color: MonacoTheme.crown, dark: "", light: ""),
    ]
}

struct GalleryColourBoard: View {
    var body: some View {
        GalleryGroup(title: "Dark and light, ratio on canvas, on cell") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                ForEach(GalleryColourRole.board) { role in
                    HStack(spacing: MonacoTheme.Space.sm) {
                        HStack(spacing: MonacoTheme.Space.sm) {
                            GallerySwatch(color: role.color, scheme: .dark)
                            GallerySwatch(color: role.color, scheme: .light)
                        }
                        VStack(alignment: .leading, spacing: 0) {
                            Text(role.name)
                                .font(MonacoTheme.Typo.captionStrong)
                                .foregroundStyle(MonacoTheme.ink)
                            GalleryCaption("dark \(GalleryHex.string(role.color, in: .dark)) \(role.dark)")
                            GalleryCaption("light \(GalleryHex.string(role.color, in: .light)) \(role.light)")
                        }
                        Spacer(minLength: 0)
                    }
                    .frame(minHeight: 44)
                    .accessibilityElement(children: .combine)
                }
            }
        }
    }
}

struct GalleryRuleTable: View {
    let title: String
    let rows: [(name: String, value: String, use: String)]

    var body: some View {
        GalleryGroup(title: title) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                ForEach(rows, id: \.name) { row in
                    VStack(alignment: .leading, spacing: 0) {
                        Text("\(row.name) · \(row.value)")
                            .font(MonacoTheme.Typo.headline)
                            .foregroundStyle(MonacoTheme.ink)
                        GalleryCaption(row.use)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .accessibilityElement(children: .combine)
                }
            }
        }
    }
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
        GalleryRuleTable(
            title: "Radii",
            rows: [
                (
                    "cell / card", "\(Int(MonacoTheme.Radius.card))",
                    "vote card, cash row, settings groups, profile card"
                ),
                ("button 50", "\(Int(MonacoTheme.Radius.button))", "prominent and bordered, full width"),
                ("share card", "\(Int(MonacoTheme.Radius.shareCard))", "the position, vote and share cards"),
                ("small", "\(Int(MonacoTheme.Radius.small))", "field, stat row, podium step, rank badge"),
                ("ticker tile · logo", "\(Int(MonacoTheme.Radius.tile)) · 15 %", "Monaco"),
                ("keypad key", "\(Int(MonacoTheme.Radius.keypadKey))", "press highlight only"),
                ("capsule", "999", "chip, reaction, toast, tab bar, search field, composer"),
                ("cabal squircle · sheet top", "22.5 % · 38", "app-icon shape · the display corner (system)"),
            ])
        GalleryRuleTable(
            title: "Spacing",
            rows: [
                (
                    "margin · card-pad", "\(Int(MonacoTheme.Space.gutter)) · \(Int(MonacoTheme.Space.cardPadding))",
                    "every screen, sheet, bar; inside cards"
                ),
                ("row", "10 × 16, min 44", "every list row, post, comment"),
                (
                    "section gap", "\(Int(MonacoTheme.Space.section)) · \(Int(MonacoTheme.Space.root)) root",
                    "above a section title; between root blocks"
                ),
                (
                    "title gap · stack gap", "\(Int(MonacoTheme.Space.titleGap)) · \(Int(MonacoTheme.Space.stackGap))",
                    "title to content; between cards"
                ),
                ("button · small", "50 · 32", ".controlSize(.large) / (.small)"),
                (
                    "tab clear · home clear",
                    "\(Int(MonacoTheme.Space.tabClear)) · \(Int(MonacoTheme.Space.homeClear))",
                    "nothing rests under the bar"
                ),
            ])
        GalleryGroup(title: "Spacing scale") {
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
            GalleryColourBoard()
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
