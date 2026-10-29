import SwiftUI

struct MonacoGlassButton: View {
    enum Style {
        case glass, quiet, selected
        case onCover
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
    var isBusy: Bool
    let action: () -> Void
    @MonacoReduceTransparency private var reduceTransparency

    @ScaledMetric(relativeTo: .body) private var regularDiameter: CGFloat = 44
    @ScaledMetric(relativeTo: .body) private var compactDiameter: CGFloat = 32
    @ScaledMetric(relativeTo: .body) private var glyphSize: CGFloat = 20

    init(
        systemImage: String,
        accessibilityLabel: String,
        style: Style = .glass,
        placement: Placement = .content,
        size: Size = .regular,
        isBusy: Bool = false,
        action: @escaping () -> Void
    ) {
        self.systemImage = systemImage
        self.accessibilityLabel = accessibilityLabel
        self.style = style
        self.placement = placement
        self.size = size
        self.isBusy = isBusy
        self.action = action
    }

    var body: some View {
        Button(action: action) {
            mark
                .frame(width: diameter, height: diameter)
                .modifier(MonacoGlassFill(style: style, drawsCircle: drawsCircle))
                .frame(minWidth: hitSide, minHeight: hitSide)
                .contentShape(Rectangle())
        }
        .buttonStyle(MonacoGlassPressStyle())
        .frame(minWidth: 44, minHeight: 44)
        .contentShape(Rectangle())
        .disabled(isBusy)
        .accessibilityLabel(accessibilityLabel)
    }

    @ViewBuilder private var mark: some View {
        if isBusy {
            ProgressView().tint(glyphColor)
        } else {
            Image(systemName: systemImage)
                .font(.system(size: min(glyphSize, 26), weight: .medium))
                .symbolVariant(style == .selected ? .fill : .none)
                .symbolRenderingMode(.monochrome)
                .foregroundStyle(glyphColor)
        }
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
        case .selected: MonacoTheme.tint
        case .onCover: MonacoTheme.primaryText
        }
    }

    private var drawsCircle: Bool {
        guard placement == .toolbar, !reduceTransparency else { return true }
        if #available(iOS 26, *) { return false }
        return true
    }
}

private struct MonacoGlassFill: ViewModifier {
    let style: MonacoGlassButton.Style
    let drawsCircle: Bool

    @MonacoReduceTransparency private var reduceTransparency
    @Environment(\.displayScale) private var displayScale

    @ViewBuilder
    func body(content: Content) -> some View {
        if !drawsCircle {
            content
        } else {
            switch style {
            case .glass, .onCover: glass(content)
            case .quiet, .selected: content.background(Circle().fill(MonacoTheme.surfaceSunken))
            }
        }
    }

    @ViewBuilder
    private func glass(_ content: Content) -> some View {
        if reduceTransparency {
            content
                .background(Circle().fill(MonacoTheme.surface))
                .overlay(Circle().strokeBorder(MonacoTheme.hairline, lineWidth: 1 / displayScale))
        } else if #available(iOS 26, *) {
            content.glassEffect(.regular.interactive(), in: .circle)
        } else {
            content
                .background(Circle().fill(.ultraThinMaterial))
                .overlay(Circle().strokeBorder(MonacoTheme.hairline, lineWidth: 1 / displayScale))
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
