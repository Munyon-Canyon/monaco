import SwiftUI

struct MonacoGlassButton: View {
    enum Style {
        case glass, quiet, selected, onCover
    }

    enum Placement {
        case toolbar, content
    }

    enum Size {
        case regular, compact
    }

    let systemImage: String
    let accessibilityLabel: String
    var style: Style
    var placement: Placement
    var size: Size
    let action: () -> Void

    @ScaledMetric(relativeTo: .body) private var regularDiameter: CGFloat = 44
    @ScaledMetric(relativeTo: .body) private var compactDiameter: CGFloat = 40
    @ScaledMetric(relativeTo: .body) private var glyphSize: CGFloat = 20

    init(
        systemImage: String,
        accessibilityLabel: String,
        style: Style = .glass,
        placement: Placement = .content,
        size: Size = .regular,
        action: @escaping () -> Void
    ) {
        self.systemImage = systemImage
        self.accessibilityLabel = accessibilityLabel
        self.style = style
        self.placement = placement
        self.size = size
        self.action = action
    }

    var body: some View {
        Button(action: action) {
            Image(systemName: systemImage)
                .font(.system(size: min(glyphSize, 26), weight: .medium))
                .symbolVariant(style == .selected ? .fill : .none)
                .symbolRenderingMode(.monochrome)
                .foregroundStyle(glyphColor)
                .frame(width: diameter, height: diameter)
                .modifier(MonacoGlassFill(style: style, drawsCircle: drawsCircle))
                .frame(width: hitSide, height: hitSide)
                .contentShape(Circle())
        }
        .buttonStyle(MonacoGlassPressStyle())
        .accessibilityLabel(accessibilityLabel)
    }

    private var diameter: CGFloat {
        min(size == .compact ? compactDiameter : regularDiameter, 56)
    }

    private var hitSide: CGFloat {
        max(diameter, 44)
    }

    private var glyphColor: Color {
        switch style {
        case .glass, .quiet: MonacoTheme.primaryText
        case .selected: MonacoTheme.brand
        case .onCover: Color.white
        }
    }

    private var drawsCircle: Bool {
        guard placement == .toolbar else { return true }
        if #available(iOS 26, *) { return false }
        return true
    }
}

private struct MonacoGlassFill: ViewModifier {
    let style: MonacoGlassButton.Style
    let drawsCircle: Bool

    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency

    @ViewBuilder
    func body(content: Content) -> some View {
        if !drawsCircle {
            content
        } else {
            switch style {
            case .glass: glass(content)
            case .quiet: content
            case .selected: content.background(Circle().fill(MonacoTheme.brandWash))
            case .onCover:
                content
                    .background(Circle().fill(Color.white.opacity(0.094)))
                    .overlay(Circle().strokeBorder(Color.white.opacity(0.14), lineWidth: 1))
            }
        }
    }

    @ViewBuilder
    private func glass(_ content: Content) -> some View {
        if reduceTransparency {
            content
                .background(Circle().fill(MonacoTheme.surface))
                .overlay(Circle().strokeBorder(MonacoTheme.border, lineWidth: 1))
        } else if #available(iOS 26, *) {
            content.glassEffect(.regular.interactive(), in: .circle)
        } else {
            content
                .background(Circle().fill(.ultraThinMaterial))
                .overlay(Circle().strokeBorder(MonacoTheme.border, lineWidth: 1))
        }
    }
}

private struct MonacoGlassPressStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.97 : 1)
            .opacity(isEnabled ? 1 : 0.45)
            .animation(.easeOut(duration: 0.2), value: configuration.isPressed)
    }
}
