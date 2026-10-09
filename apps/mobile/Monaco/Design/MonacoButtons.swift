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

/// Shared button geometry. `MonacoToastPlacement` sizes its inset against this, so the bar height
/// and the toast's idea of the bar height cannot drift apart.
enum MonacoButtonMetrics {
    static let minimumHeight: CGFloat = 56

    static let compactHeight: CGFloat = 36

    /// A disabled button is the same fill at under half its strength, its label at 90%.
    static let disabledFillOpacity = 0.45
}

private struct MonacoButtonChrome: ViewModifier {
    let fill: Color
    let label: Color
    let isPressed: Bool
    var isCompact = false
    var tapsOnPress = false

    @Environment(\.isEnabled) private var isEnabled
    @Environment(\.monacoButtonFullWidth) private var fullWidth

    func body(content: Content) -> some View {
        content
            .font(isCompact ? MonacoTheme.Typo.subheadStrong : MonacoTheme.Typo.headline)
            .lineLimit(1)
            .minimumScaleFactor(0.9)
            .foregroundStyle(label.opacity(isEnabled ? 1 : 0.9))
            .padding(.horizontal, isCompact ? MonacoTheme.Space.m : MonacoTheme.Space.l)
            .frame(
                maxWidth: fullWidth && !isCompact ? .infinity : nil,
                minHeight: isCompact ? MonacoButtonMetrics.compactHeight : MonacoButtonMetrics.minimumHeight
            )
            .background(Capsule().fill(fill.opacity(isEnabled ? 1 : MonacoButtonMetrics.disabledFillOpacity)))
            .frame(minHeight: 44)
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

struct MonacoDestructiveButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.modifier(
            MonacoButtonChrome(
                fill: MonacoTheme.surfaceSunken, label: MonacoTheme.destructive, isPressed: configuration.isPressed))
    }
}

/// Content width, 36pt visible on a 44pt target; the prominent one is a row's single compact primary.
struct MonacoCompactButtonStyle: ButtonStyle {
    var isProminent = false

    func makeBody(configuration: Configuration) -> some View {
        configuration.label.modifier(
            MonacoButtonChrome(
                fill: isProminent ? MonacoTheme.primaryButtonFill : MonacoTheme.secondaryButtonFill,
                label: isProminent ? MonacoTheme.primaryButtonLabel : MonacoTheme.secondaryButtonLabel,
                isPressed: configuration.isPressed, isCompact: true))
    }
}

extension ButtonStyle where Self == MonacoPrimaryButtonStyle {
    static var monacoPrimary: MonacoPrimaryButtonStyle { MonacoPrimaryButtonStyle() }
}

extension ButtonStyle where Self == MonacoSecondaryButtonStyle {
    static var monacoSecondary: MonacoSecondaryButtonStyle { MonacoSecondaryButtonStyle() }
}

extension ButtonStyle where Self == MonacoDestructiveButtonStyle {
    static var monacoDestructive: MonacoDestructiveButtonStyle { MonacoDestructiveButtonStyle() }
}

extension ButtonStyle where Self == MonacoCompactButtonStyle {
    static var monacoCompact: MonacoCompactButtonStyle { MonacoCompactButtonStyle() }
    static var monacoCompactProminent: MonacoCompactButtonStyle { MonacoCompactButtonStyle(isProminent: true) }
}

struct MonacoTextButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: ButtonStyleConfiguration) -> some View {
        configuration.label
            .font(MonacoTheme.Typo.subheadStrong)
            .foregroundStyle(isEnabled ? MonacoTheme.brand : MonacoTheme.disabledLabel)
            .frame(minWidth: 44, minHeight: 44)
            .contentShape(Rectangle())
            .opacity(configuration.isPressed ? 0.6 : 1)
    }
}

extension ButtonStyle where Self == MonacoTextButtonStyle {
    static var monacoText: MonacoTextButtonStyle { MonacoTextButtonStyle() }
}

struct SubmitLabel: View {
    let isWorking: Bool
    let idle: String
    let working: String

    init(isWorking: Bool, idle: String, working: String) {
        self.isWorking = isWorking
        self.idle = idle
        self.working = working
    }

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            if isWorking {
                ProgressView().tint(MonacoTheme.primaryButtonLabel)
            }
            Text(isWorking ? working : idle)
        }
        .frame(maxWidth: .infinity)
    }
}

/// Pinned action bar. Use inside `.safeAreaInset(edge: .bottom) { BottomCTA { … } }` so it rides above the keyboard.
/// One primary, or a primary and a secondary side by side; they stack at accessibility sizes.
struct BottomCTA<Content: View>: View {
    private let content: Content
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.displayScale) private var displayScale
    @Environment(ToastCenter.self) private var toasts: ToastCenter?

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        layout {
            content
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .padding(.bottom, MonacoTheme.Space.s)
        .background { MonacoTheme.canvas.ignoresSafeArea(edges: .bottom) }
        .overlay(alignment: .top) {
            Rectangle()
                .fill(MonacoTheme.hairline)
                .frame(height: 1 / displayScale)
        }
        .onAppear { toasts?.bottomCTAs += 1 }
        .onDisappear { toasts?.bottomCTAs -= 1 }
    }
}

/// Ceilings for `CircleAction`'s disc and glyph.
///
/// The disc is a container, not type, and it shares a row with three others: `GroupActionRow` puts
/// four `CircleAction`s in an `HStack(spacing: 0)` with each in a `.frame(maxWidth: .infinity)`
/// column — about 97pt on a 390pt screen. Scaled without a ceiling, a 56pt disc reaches ~99pt at
/// AX1 and ~189pt at AX5, so the four main money actions draw over each other from AX1 upwards.
/// A 2x2 grid would only raise the column to ~195pt, so it does not remove the need for a ceiling.
///
/// The glyph keeps scaling up to its own ceiling, so the control still grows with Dynamic Type —
/// it just stops growing before it outgrows the space it has. Past the ceiling the label below,
/// which is uncapped, carries the rest of the size increase.
enum CircleActionMetrics {
    /// Fits the ~97pt column the four-across row gives each action on the narrowest phone.
    static let maximumDiscSize: CGFloat = 88

    /// Keeps the symbol proportionate inside a capped disc (20/56 of the disc, as at the base size).
    static let maximumGlyphSize: CGFloat = 30

    static func discSize(scaled: CGFloat) -> CGFloat { min(scaled, maximumDiscSize) }

    static func glyphSize(scaled: CGFloat) -> CGFloat { min(scaled, maximumGlyphSize) }
}

/// Brand-tinted disc with a symbol and a footnote label below (Group detail action row).
/// A wash rather than a solid fill: four solid brand discs in a row would spend the accent.
/// The disc, the glyph and the label all scale with Dynamic Type — these are the main money
/// actions, and they used to stay at 13pt while every label around them grew. The disc and the
/// glyph stop at `CircleActionMetrics`' ceilings so they stay inside their column; the label does not.
struct CircleAction: View {
    private let title: String
    private let systemImage: String
    private let action: () -> Void

    @Environment(\.isEnabled) private var isEnabled
    @ScaledMetric(relativeTo: .footnote) private var scaledDiscSize: CGFloat = 56
    @ScaledMetric(relativeTo: .footnote) private var scaledGlyphSize: CGFloat = 20

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
                    .font(.system(size: glyphSize, weight: .semibold))
                    // Filled, not washed. The brand is monochrome now, so a low-alpha brand tint
                    // over cream has no colour left in it — the disc came out a flat sage-grey and
                    // the row read as four disabled controls. A solid disc carries the affordance
                    // the way the saturated blue glyph used to.
                    .foregroundStyle(isEnabled ? MonacoTheme.onBrand : MonacoTheme.disabledLabel)
                    .frame(width: discSize, height: discSize)
                    .background(Circle().fill(isEnabled ? MonacoTheme.brandFill : MonacoTheme.surfaceSunken))
                Text(title)
                    .font(MonacoTheme.Typo.caption)

                    .foregroundStyle(isEnabled ? MonacoTheme.ink : MonacoTheme.disabledLabel)
                    .lineLimit(2)
                    .multilineTextAlignment(.center)
                    .minimumScaleFactor(0.8)
            }
            .frame(minWidth: 64)
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
