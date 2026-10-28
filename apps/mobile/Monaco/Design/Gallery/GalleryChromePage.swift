#if DEBUG
import SwiftUI

struct GalleryChromePage: View {
    var body: some View {
        GalleryPage(title: "Chrome") {
            GalleryGlassSamples()
            GalleryToolbarSamples()
            GalleryTabBarSamples()
            GalleryChromeDockSamples()
            GallerySheetSamples()
        }
    }
}

struct GalleryGlassSamples: View {
    private let styles: [(name: String, style: MonacoGlassButton.Style, icon: MonacoIcon)] = [
        ("glass", .glass, .share), ("quiet", .quiet, .close), ("selected", .selected, .star),
    ]

    private let sizes: [(name: String, size: MonacoGlassButton.Size, reduceTransparency: Bool)] = [
        ("Size 44", .regular, false), ("Size compact 32 on 44", .compact, false),
        ("Size 44, Reduce Transparency", .regular, true), ("Size compact, Reduce Transparency", .compact, true),
    ]

    var body: some View {
        GalleryGroup(title: "Glass buttons") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                ForEach(sizes, id: \.name) { entry in
                    GalleryCase(caption: entry.name) {
                        GalleryGrid(minimum: 72) {
                            ForEach(styles, id: \.name) { sample in
                                VStack(spacing: MonacoTheme.Space.xs) {
                                    ZStack {
                                        backdrop
                                        MonacoGlassButton(
                                            icon: sample.icon, accessibilityLabel: sample.name,
                                            style: sample.style, size: entry.size
                                        ) {}
                                    }
                                    .environment(\.monacoReduceTransparencyOverride, entry.reduceTransparency)
                                    .frame(width: 64, height: 64)
                                    GalleryCaption(sample.name)
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    private var backdrop: some View {
        RoundedRectangle(cornerRadius: MonacoTheme.Radius.badge, style: .continuous)
            .fill(MonacoTheme.CabalTint.blue.fill)
    }
}

struct GalleryToolbarSample: View {
    let title: String
    let size: MonacoGlassButton.Size

    var body: some View {
        NavigationStack {
            MonacoTheme.canvas
                .navigationTitle(title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) {
                        MonacoGlassButton(
                            systemImage: "chevron.left", accessibilityLabel: "Back", style: .glass,
                            placement: .toolbar, size: size
                        ) {}
                    }
                    ToolbarItemGroup(placement: .topBarTrailing) {
                        MonacoGlassButton(
                            icon: .share, accessibilityLabel: "Share", style: .quiet, placement: .toolbar,
                            size: size
                        ) {}
                        MonacoGlassButton(
                            icon: .star, accessibilityLabel: "Watch", style: .selected, placement: .toolbar,
                            size: size
                        ) {}
                    }
                }
        }
        .frame(height: 120)
        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                .strokeBorder(MonacoTheme.hairline, lineWidth: 1))
    }
}

struct GalleryToolbarSamples: View {
    var body: some View {
        GalleryGroup(title: "Toolbar items") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Glass, quiet and selected in a toolbar, size 44") {
                    GalleryToolbarSample(title: "Cabal", size: .regular)
                }
                GalleryCase(caption: "Glass, quiet and selected in a toolbar, compact 32 on 44") {
                    GalleryToolbarSample(title: "Cabal", size: .compact)
                }
            }
        }
    }
}

struct GalleryTabBarSamples: View {
    var body: some View {
        GalleryGroup(title: "Tab bar") {
            GalleryCase(caption: "Five tabs, Cabals badge 1") {
                GalleryTabBarSample()
            }
        }
    }
}

struct GalleryChromeDockSamples: View {
    var body: some View {
        GalleryGroup(title: "Docked bar") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Docked Deposit on the bar material") {
                    dock
                }
                GalleryCase(caption: "Docked Deposit, Reduce Transparency") {
                    dock.environment(\.monacoReduceTransparencyOverride, true)
                }
            }
        }
    }

    private var dock: some View {
        BottomCTA {
            Button("Deposit") {}.buttonStyle(.monacoPrimary)
        }
        .monacoBleed()
    }
}

enum GallerySheetKind: String, CaseIterable, Identifiable {
    case review, votes, result

    var id: String { rawValue }

    var title: String {
        switch self {
        case .review: "Review proposal"
        case .votes: "Votes"
        case .result: "Deposit confirmed"
        }
    }

    var detents: MonacoSheetDetents {
        switch self {
        case .review: .review
        case .votes: .votes
        case .result: .result
        }
    }
}

struct GallerySheetSamples: View {
    @State private var presented: GallerySheetKind?

    var body: some View {
        GalleryGroup(title: "Sheet") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Sheet heading with close") {
                    MonacoSheetHeading(title: "Review proposal", identifier: nil, showsClose: true)
                        .monacoBleed()
                }
                GalleryCase(caption: "Sheets by detent kind: review, votes, result") {
                    GalleryGrid(minimum: 96) {
                        ForEach(GallerySheetKind.allCases) { kind in
                            Button(kind.rawValue.capitalized) { presented = kind }
                                .buttonStyle(.monacoCompact)
                                .accessibilityLabel("Open \(kind.rawValue) sheet")
                        }
                    }
                }
            }
        }
        .sheet(item: $presented) { kind in
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCaption("Detent kind: \(kind.rawValue)")
                Spacer(minLength: 0)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .frame(maxWidth: .infinity, alignment: .leading)
            .monacoSheet(title: kind.title, detents: kind.detents, showsClose: true)
        }
    }
}
#endif
