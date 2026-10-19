import SwiftUI

struct CabalCover<Overlay: View>: View {
    enum Style: Equatable {
        case hero, heroTall, card, memberCard
        case thumb(CGFloat)
    }

    let cabalId: String
    let tint: MonacoTheme.CabalTint
    let pictureURL: URL?
    let style: Style
    private let overlay: Overlay

    @State private var loadedPicture: UIImage?
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency
    @Environment(\.displayScale) private var displayScale

    private static var heroHeight: CGFloat { 132 }
    private static var heroTallHeight: CGFloat { 172 }
    private static var gradientDarkening: CGFloat { 0.55 }
    private static var gradientEnd: Color { Color(hex: 0x000C08) }

    init(
        cabalId: String,
        tint: MonacoTheme.CabalTint,
        pictureURL: URL?,
        style: Style,
        @ViewBuilder overlay: () -> Overlay
    ) {
        self.cabalId = cabalId
        self.tint = tint
        self.pictureURL = pictureURL
        self.style = style
        self.overlay = overlay()
    }

    static func cornerRadius(for style: Style) -> CGFloat {
        switch style {
        case .hero, .heroTall: return MonacoTheme.Radius.identity
        case .card: return MonacoTheme.Radius.cardTight
        case .memberCard: return MonacoTheme.Radius.card
        case .thumb(let size): return MarkGeometry.radius(for: size)
        }
    }

    static func scrim(for style: Style, reduceTransparency: Bool) -> MonacoTheme.CoverScrim? {
        switch style {
        case .thumb: return nil
        case _ where reduceTransparency: return .solid
        case .hero, .heroTall: return .hero
        case .card: return .card
        case .memberCard: return .memberCard
        }
    }

    private var shape: RoundedRectangle {
        RoundedRectangle(cornerRadius: Self.cornerRadius(for: style), style: .continuous)
    }

    var body: some View {
        sized
            .background { layers }
            .clipShape(shape)
            .task(id: pictureURL) { await loadPicture() }
    }

    @ViewBuilder
    private var sized: some View {
        switch style {
        case .hero:
            overlay.frame(maxWidth: .infinity, minHeight: Self.heroHeight, alignment: .bottomLeading)
        case .heroTall:
            overlay.frame(maxWidth: .infinity, minHeight: Self.heroTallHeight, alignment: .bottomLeading)
        case .card, .memberCard:
            overlay.frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomLeading)
        case .thumb(let size):
            Color.clear.frame(width: size, height: size)
        }
    }

    private var layers: some View {
        ZStack {
            LinearGradient(
                colors: [tint.fill, tint.fill.mix(with: Self.gradientEnd, by: Self.gradientDarkening)],
                startPoint: .topLeading,
                endPoint: .bottomTrailing)
            Color.clear.overlay {
                if let picture = loadedPicture ?? cachedPicture {
                    Image(uiImage: picture)
                        .resizable()
                        .scaledToFill()
                }
            }
            .clipped()
            if let scrim = Self.scrim(for: style, reduceTransparency: reduceTransparency) {
                LinearGradient(gradient: scrim.gradient, startPoint: .top, endPoint: .bottom)
            }
            if style.drawsStroke {
                shape.strokeBorder(MonacoTheme.coverStroke, lineWidth: 1 / displayScale)
            }
        }
        .accessibilityHidden(true)
    }

    private var cachedPicture: UIImage? {
        pictureURL.flatMap { MonacoRemoteImageStore.avatars.cachedImage(for: $0) }
    }

    private func loadPicture() async {
        loadedPicture = nil
        guard let pictureURL else { return }
        let store = MonacoRemoteImageStore.avatars
        if let cached = store.cachedImage(for: pictureURL) {
            loadedPicture = cached
            return
        }
        let fetched = await store.image(for: pictureURL)
        guard !Task.isCancelled, let fetched else { return }
        withAnimation(.easeOut(duration: 0.2)) { loadedPicture = fetched }
    }
}

extension CabalCover.Style {
    fileprivate var drawsStroke: Bool {
        if case .thumb = self { return false }
        return true
    }
}

extension CabalCover where Overlay == EmptyView {
    init(cabalId: String, tint: MonacoTheme.CabalTint, pictureURL: URL?, style: Style) {
        self.init(cabalId: cabalId, tint: tint, pictureURL: pictureURL, style: style) { EmptyView() }
    }
}

struct CoverShareDisc: View {
    let cabalName: String
    let action: () -> Void

    private static let diameter: CGFloat = 40
    private static let hit: CGFloat = 44
    private static let glyph: CGFloat = 17

    @Environment(\.displayScale) private var displayScale

    var body: some View {
        Button(action: action) {
            Image(monaco: .share)
                .font(.system(size: Self.glyph, weight: .medium))
                .foregroundStyle(MonacoTheme.onCover)
                .frame(width: Self.diameter, height: Self.diameter)
                .background(MonacoTheme.onCoverGlass, in: Circle())
                .overlay { Circle().strokeBorder(MonacoTheme.onCoverGlassStroke, lineWidth: 1 / displayScale) }
                .frame(width: Self.hit, height: Self.hit)
                .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Share \(cabalName)")
    }
}
