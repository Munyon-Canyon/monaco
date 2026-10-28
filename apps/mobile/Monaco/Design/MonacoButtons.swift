import SwiftUI

/// Shared press feel: scale 0.97 on a short spring, none under Reduce Motion.
private struct MonacoPressEffect: ViewModifier {
    let isPressed: Bool
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        content
            .scaleEffect(isPressed && !reduceMotion ? 0.97 : 1)
            .opacity(isPressed && reduceMotion ? 0.8 : 1)
            .animation(reduceMotion ? nil : .spring(response: 0.25, dampingFraction: 0.8), value: isPressed)
    }
}

private struct MonacoButtonFullWidthKey: EnvironmentKey {
    static let defaultValue = true
}

extension EnvironmentValues {
    var monacoButtonFullWidth: Bool {
        get { self[MonacoButtonFullWidthKey.self] }
        set { self[MonacoButtonFullWidthKey.self] = newValue }
    }
}

extension View {
    /// Full width is the default for the monaco button styles; `false` opts out and sizes the capsule to its label.
    func monacoFullWidthButtons(_ enabled: Bool = true) -> some View {
        environment(\.monacoButtonFullWidth, enabled)
    }
}

enum MonacoButtonMetrics {
    static let minimumHeight: CGFloat = 50
    static let horizontalPadding: CGFloat = 16
    static let verticalPadding: CGFloat = 10
    static let iconSize: CGFloat = 17
    static let iconGap: CGFloat = 8
    static let compactHeight: CGFloat = 32
    static let compactMinWidth: CGFloat = 82
    static let hitTarget: CGFloat = 44
    static let disabledFillOpacity = 0.45
}

private enum MonacoButtonKind {
    case regular
    case small
    case deposit

    var visualHeight: CGFloat {
        switch self {
        case .regular: MonacoButtonMetrics.minimumHeight
        case .small, .deposit: MonacoButtonMetrics.compactHeight
        }
    }

    var radius: CGFloat {
        self == .regular ? MonacoTheme.Radius.button : MonacoTheme.Radius.small
    }

    var horizontalPadding: CGFloat {
        switch self {
        case .regular: MonacoButtonMetrics.horizontalPadding
        case .small: MonacoTheme.Space.sm
        case .deposit: MonacoTheme.Space.m
        }
    }

    var verticalPadding: CGFloat {
        self == .regular ? MonacoButtonMetrics.verticalPadding : MonacoTheme.Space.xs
    }

    var font: Font {
        self == .regular ? MonacoTheme.Typo.button : MonacoTheme.Typo.subheadStrong
    }
}

private struct MonacoButtonLabelStyle: LabelStyle {
    @ScaledMetric(relativeTo: .body) private var iconSize = MonacoButtonMetrics.iconSize

    func makeBody(configuration: Configuration) -> some View {
        HStack(spacing: MonacoButtonMetrics.iconGap) {
            configuration.icon.font(.system(size: iconSize, weight: .semibold))
            configuration.title
        }
    }
}

private struct MonacoButtonChrome: ViewModifier {
    let fill: Color
    let label: Color
    let isPressed: Bool
    var kind = MonacoButtonKind.regular
    var tapsOnPress = false

    @Environment(\.isEnabled) private var isEnabled
    @Environment(\.monacoButtonFullWidth) private var fullWidth
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    func body(content: Content) -> some View {
        content
            .labelStyle(MonacoButtonLabelStyle())
            .font(kind.font)
            .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 1)
            .minimumScaleFactor(0.9)
            .multilineTextAlignment(.center)
            .foregroundStyle(isEnabled ? label : MonacoTheme.disabledLabel)
            .padding(.horizontal, kind.horizontalPadding)
            .padding(.vertical, kind.verticalPadding)
            .frame(
                minWidth: kind == .deposit ? MonacoButtonMetrics.compactMinWidth : nil,
                maxWidth: fullWidth && kind == .regular ? .infinity : nil,
                minHeight: kind.visualHeight
            )
            .background(
                RoundedRectangle(cornerRadius: kind.radius, style: .continuous)
                    .fill(isEnabled ? fill : MonacoTheme.surfaceSunken)
            )
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(Rectangle())
            .modifier(MonacoPressEffect(isPressed: isPressed))
            .sensoryFeedback(.impact(weight: .light), trigger: isPressed) { _, pressed in tapsOnPress && pressed }
    }
}

struct MonacoPrimaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.modifier(
            MonacoButtonChrome(
                fill: MonacoTheme.primaryButtonFill, label: MonacoTheme.primaryButtonLabel,
                isPressed: configuration.isPressed, tapsOnPress: true))
    }
}

struct MonacoSecondaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.modifier(
            MonacoButtonChrome(
                fill: MonacoTheme.secondaryButtonFill, label: MonacoTheme.secondaryButtonLabel,
                isPressed: configuration.isPressed))
    }
}

struct MonacoOutlineButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        MonacoSecondaryButtonStyle().makeBody(configuration: configuration)
    }
}

struct MonacoDestructiveButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.modifier(
            MonacoButtonChrome(
                fill: MonacoTheme.secondaryButtonFill, label: MonacoTheme.loss, isPressed: configuration.isPressed))
    }
}

struct MonacoCompactButtonStyle: ButtonStyle {
    var isProminent = false

    func makeBody(configuration: Configuration) -> some View {
        configuration.label.modifier(
            MonacoButtonChrome(
                fill: isProminent ? MonacoTheme.primaryButtonFill : MonacoTheme.secondaryButtonFill,
                label: isProminent ? MonacoTheme.primaryButtonLabel : MonacoTheme.secondaryButtonLabel,
                isPressed: configuration.isPressed, kind: isProminent ? .deposit : .small))
    }
}

extension ButtonStyle where Self == MonacoPrimaryButtonStyle {
    static var monacoPrimary: MonacoPrimaryButtonStyle { MonacoPrimaryButtonStyle() }
}

extension ButtonStyle where Self == MonacoSecondaryButtonStyle {
    static var monacoSecondary: MonacoSecondaryButtonStyle { MonacoSecondaryButtonStyle() }
}

extension ButtonStyle where Self == MonacoOutlineButtonStyle {
    static var monacoOutline: MonacoOutlineButtonStyle { MonacoOutlineButtonStyle() }
}

extension ButtonStyle where Self == MonacoDestructiveButtonStyle {
    static var monacoDestructive: MonacoDestructiveButtonStyle { MonacoDestructiveButtonStyle() }
}

extension ButtonStyle where Self == MonacoCompactButtonStyle {
    static var monacoCompact: MonacoCompactButtonStyle { MonacoCompactButtonStyle() }
    static var monacoCompactProminent: MonacoCompactButtonStyle { MonacoCompactButtonStyle(isProminent: true) }
}

struct MonacoTextButtonStyle: ButtonStyle {
    var showsChevron = false

    @Environment(\.isEnabled) private var isEnabled
    @ScaledMetric(relativeTo: .footnote) private var chevronSize: CGFloat = 13

    func makeBody(configuration: ButtonStyleConfiguration) -> some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            configuration.label
            if showsChevron {
                Image(systemName: "chevron.right")
                    .font(.system(size: chevronSize, weight: .semibold))
                    .accessibilityHidden(true)
            }
        }
        .font(MonacoTheme.Typo.subhead)
        .foregroundStyle(isEnabled ? MonacoTheme.tint : MonacoTheme.disabledLabel)
        .frame(minWidth: MonacoButtonMetrics.hitTarget, minHeight: MonacoButtonMetrics.hitTarget)
        .contentShape(Rectangle())
        .opacity(configuration.isPressed ? 0.6 : 1)
    }
}

extension ButtonStyle where Self == MonacoTextButtonStyle {
    static var monacoText: MonacoTextButtonStyle { MonacoTextButtonStyle() }
    static func monacoText(chevron: Bool) -> MonacoTextButtonStyle { MonacoTextButtonStyle(showsChevron: chevron) }
}

struct MonacoLoadMoreButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: ButtonStyleConfiguration) -> some View {
        configuration.label
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(isEnabled ? MonacoTheme.secondaryText : MonacoTheme.disabledLabel)
            .multilineTextAlignment(.center)
            .frame(maxWidth: .infinity, minHeight: MonacoButtonMetrics.minimumHeight)
            .contentShape(Rectangle())
            .opacity(configuration.isPressed ? 0.6 : 1)
    }
}

extension ButtonStyle where Self == MonacoLoadMoreButtonStyle {
    static var monacoLoadMore: MonacoLoadMoreButtonStyle { MonacoLoadMoreButtonStyle() }
}

struct SubmitLabel: View {
    let isWorking: Bool
    let idle: String
    let working: String
    let tint: Color

    init(isWorking: Bool, idle: String, working: String, tint: Color = MonacoTheme.primaryButtonLabel) {
        self.isWorking = isWorking
        self.idle = idle
        self.working = working
        self.tint = tint
    }

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            if isWorking {
                ProgressView().tint(tint)
            }
            Text(isWorking ? working : idle)
        }
        .frame(maxWidth: .infinity)
    }
}

private struct MonacoDockBackground: View {
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency

    @ViewBuilder
    var body: some View {
        if reduceTransparency {
            Rectangle().fill(MonacoTheme.surface)
        } else if #available(iOS 26, *) {
            Color.clear.glassEffect(.regular, in: .rect)
        } else {
            Rectangle().fill(.ultraThinMaterial)
        }
    }
}

struct BottomCTA<Content: View>: View {
    private let caption: String?
    private let content: Content
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.displayScale) private var displayScale

    init(caption: String? = nil, @ViewBuilder content: () -> Content) {
        self.caption = caption
        self.content = content()
    }

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        VStack(spacing: MonacoTheme.Space.s) {
            layout {
                content
            }
            if let caption {
                Text(caption)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: .infinity)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .padding(.bottom, MonacoTheme.Space.s)
        .background { MonacoDockBackground().ignoresSafeArea(edges: .bottom) }
        .overlay(alignment: .top) {
            Rectangle()
                .fill(MonacoTheme.border)
                .frame(height: 1 / displayScale)
        }
    }
}

/// Ceilings for `CircleAction`'s disc and glyph.
///
enum CircleActionMetrics {
    static let baseDiscSize: CGFloat = 44

    static let baseGlyphSize: CGFloat = 22

    static let maximumDiscSize: CGFloat = 64

    static let maximumGlyphSize: CGFloat = 30

    static let minimumTileWidth: CGFloat = 66

    static let minimumTileHeight: CGFloat = 72

    static let disabledOpacity = 0.35

    static let maximumLabelSize = DynamicTypeSize.accessibility2

    static func discSize(scaled: CGFloat) -> CGFloat { min(scaled, maximumDiscSize) }

    static func glyphSize(scaled: CGFloat) -> CGFloat { min(scaled, maximumGlyphSize) }
}

struct CircleAction: View {
    private let title: String
    private let systemImage: String
    private let action: () -> Void

    @Environment(\.isEnabled) private var isEnabled
    @ScaledMetric(relativeTo: .footnote) private var scaledDiscSize = CircleActionMetrics.baseDiscSize
    @ScaledMetric(relativeTo: .footnote) private var scaledGlyphSize = CircleActionMetrics.baseGlyphSize

    private var discSize: CGFloat { CircleActionMetrics.discSize(scaled: scaledDiscSize) }

    private var glyphSize: CGFloat { CircleActionMetrics.glyphSize(scaled: scaledGlyphSize) }

    init(_ title: String, systemImage: String, action: @escaping () -> Void) {
        self.title = title
        self.systemImage = systemImage
        self.action = action
    }

    var body: some View {
        Button {
            Haptics.tap()
            action()
        } label: {
            VStack(spacing: MonacoTheme.Space.s) {
                Image(systemName: systemImage)
                    .font(.system(size: glyphSize, weight: .medium))
                    .foregroundStyle(MonacoTheme.brand)
                    .frame(width: discSize, height: discSize)
                    .background(Circle().fill(MonacoTheme.brandSoft))
                Text(title)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(2)
                    .multilineTextAlignment(.center)
                    .minimumScaleFactor(0.8)
                    .dynamicTypeSize(...CircleActionMetrics.maximumLabelSize)
            }
            .frame(
                minWidth: CircleActionMetrics.minimumTileWidth,
                minHeight: CircleActionMetrics.minimumTileHeight
            )
            .opacity(isEnabled ? 1 : CircleActionMetrics.disabledOpacity)
            .contentShape(Rectangle())
        }
        .buttonStyle(CircleActionPressStyle())
        .accessibilityLabel(title)
    }
}

private struct CircleActionPressStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .modifier(MonacoPressEffect(isPressed: configuration.isPressed))
    }
}
