#if DEBUG
import SwiftUI
import UIKit

struct GalleryPage<Content: View>: View {
    let title: String
    @ViewBuilder let content: Content

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.section) {
                content
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .monacoCanvas()
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
    }
}

struct GalleryGroup<Content: View>: View {
    let title: String
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            MonacoSectionHeader(title)
            content
        }
    }
}

struct GalleryCase<Content: View>: View {
    let caption: String
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            content
            GalleryCaption(caption)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct GalleryCaption: View {
    let text: String

    init(_ text: String) {
        self.text = text
    }

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
            .fixedSize(horizontal: false, vertical: true)
    }
}

struct GalleryGrid<Content: View>: View {
    let minimum: CGFloat
    @ViewBuilder let content: Content

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let width = dynamicTypeSize.isAccessibilitySize ? 280 : minimum
        LazyVGrid(
            columns: [GridItem(.adaptive(minimum: width), spacing: MonacoTheme.Space.sm, alignment: .topLeading)],
            alignment: .leading, spacing: MonacoTheme.Space.sm
        ) {
            content
        }
    }
}

enum GalleryHex {
    static func string(_ color: Color, in style: UIUserInterfaceStyle) -> String {
        let resolved = UIColor(color).resolvedColor(with: UITraitCollection(userInterfaceStyle: style))
        var red: CGFloat = 0
        var green: CGFloat = 0
        var blue: CGFloat = 0
        var alpha: CGFloat = 0
        resolved.getRed(&red, green: &green, blue: &blue, alpha: &alpha)
        let hex = String(
            format: "#%02X%02X%02X", Int((red * 255).rounded()), Int((green * 255).rounded()),
            Int((blue * 255).rounded()))
        return alpha < 0.995 ? "\(hex) \(Int((alpha * 100).rounded()))%" : hex
    }
}

enum GalleryFixtures {
    static let cabalNames: [(id: String, name: String)] = [
        ("8f1c2d3e-0001", "Weekend investors"),
        ("8f1c2d3e-0002", "Semis or bust"),
        ("8f1c2d3e-0003", "Index huggers"),
        ("8f1c2d3e-0004", "Long shots"),
        ("8f1c2d3e-0005", "The extremely long cabal name for testing"),
    ]

    private static var pictures: [String: URL] = [:]

    static func picture(_ tint: MonacoTheme.CabalTint, name: String) -> URL? {
        if let known = pictures[name] { return known }
        let made = SampleImage.flat(tint, name: name)
        pictures[name] = made
        return made
    }
}

extension MonacoTheme.CabalTint {
    var galleryName: String {
        switch self {
        case .indigo: "indigo"
        case .orange: "orange"
        case .teal: "teal"
        case .pink: "pink"
        case .purple: "purple"
        case .brown: "brown"
        case .blue: "blue"
        case .cyan: "cyan"
        }
    }
}
#endif
