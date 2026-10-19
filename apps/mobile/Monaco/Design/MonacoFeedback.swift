import SwiftUI
import UIKit

/// Haptic vocabulary. `selection` for chips, segments and tabs; `tap` for primary buttons only, which play it
/// on press themselves (other styles are silent); `success` when something lands (vote, proposal, funding,
/// cash out, comment, message); `warning` on every error toast. No-ops without a Taptic Engine.
enum Haptics {
    static func tap() {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
    }

    static func success() {
        UINotificationFeedbackGenerator().notificationOccurred(.success)
    }

    static func warning() {
        UINotificationFeedbackGenerator().notificationOccurred(.warning)
    }

    static func selection() {
        UISelectionFeedbackGenerator().selectionChanged()
    }
}

/// Placeholder bar in the shape of the content that is loading.
struct SkeletonBlock: View {
    private let width: CGFloat?
    private let height: CGFloat?
    private let radius: CGFloat

    init(width: CGFloat? = nil, height: CGFloat?, radius: CGFloat = MonacoTheme.Radius.field) {
        self.width = width
        self.height = height
        self.radius = radius
    }

    var body: some View {
        RoundedRectangle(cornerRadius: radius, style: .continuous)
            .fill(MonacoTheme.surfaceSunken)
            .frame(width: width, height: height)
            .frame(maxWidth: width == nil ? .infinity : nil, alignment: .leading)
            .modifier(SkeletonPulse(active: true))
            .accessibilityHidden(true)
    }
}

enum SkeletonMetrics {
    static let titleHeight: CGFloat = 42
    static let titleWidthFraction: CGFloat = 0.7
    static let titleRadius: CGFloat = 6
    static let barHeight: CGFloat = 16
    static let barRadius: CGFloat = 6
    static let cardHeight: CGFloat = 126
    static let lineHeight: CGFloat = 14
    static let lineRadius: CGFloat = 5
    static let rowTitleFraction: CGFloat = 0.6
    static let rowSubtitleFraction: CGFloat = 0.4
    static let rowTrailingWidth: CGFloat = 56
}

struct SkeletonFraction<Content: View>: View {
    private let fraction: CGFloat
    private let content: Content

    init(_ fraction: CGFloat, @ViewBuilder content: () -> Content) {
        self.fraction = fraction
        self.content = content()
    }

    var body: some View {
        FractionLayout(fraction: fraction) { content }
    }
}

private struct FractionLayout: Layout {
    let fraction: CGFloat

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        guard let subview = subviews.first else { return .zero }
        guard let width = proposal.width else { return subview.sizeThatFits(.unspecified) }
        let scaled = width * fraction
        return CGSize(width: scaled, height: subview.sizeThatFits(ProposedViewSize(width: scaled, height: nil)).height)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        subviews.first?.place(
            at: bounds.origin, anchor: .topLeading,
            proposal: ProposedViewSize(width: bounds.width, height: bounds.height))
    }
}

struct SkeletonTitle: View {
    var body: some View {
        SkeletonFraction(SkeletonMetrics.titleWidthFraction) {
            SkeletonBlock(height: SkeletonMetrics.titleHeight, radius: SkeletonMetrics.titleRadius)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct SkeletonBar: View {
    var width: CGFloat?

    var body: some View {
        SkeletonBlock(width: width, height: SkeletonMetrics.barHeight, radius: SkeletonMetrics.barRadius)
    }
}

struct SkeletonCardBlock: View {
    var body: some View {
        SkeletonBlock(height: SkeletonMetrics.cardHeight, radius: MonacoTheme.Radius.card)
    }
}

extension View {
    func skeletonBar(width: CGFloat, radius: CGFloat = MonacoTheme.Radius.field) -> some View {
        hidden().lineLimit(1).frame(width: width).overlay { SkeletonBlock(width: width, height: nil, radius: radius) }
    }

    /// Redacts the view to placeholder shapes and pulses it while `active`.
    /// Pair with sample content of the real layout; VoiceOver hears "Loading".
    func skeleton(_ active: Bool) -> some View {
        modifier(SkeletonModifier(active: active))
    }
}

private struct SkeletonModifier: ViewModifier {
    let active: Bool

    func body(content: Content) -> some View {
        if active {
            content
                .redacted(reason: .placeholder)
                .modifier(SkeletonPulse(active: true))
                .allowsHitTesting(false)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading")
        } else {
            content
        }
    }
}

/// 1.2s opacity pulse 0.55 ↔ 1. Static under Reduce Motion.
private struct SkeletonPulse: ViewModifier {
    let active: Bool
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        content.opacityLoop(to: 0.55, halfPeriod: 1.2, active: active && !reduceMotion)
    }
}

extension View {
    /// Loops this view's opacity between 1 and `low`, easing each way over `halfPeriod`, while `active`.
    ///
    /// The loop is a `PhaseAnimator`, so its animation only ever reaches the opacity. Starting a loop with
    /// `withAnimation(.repeatForever)` in `onAppear` instead puts every change of that first update into
    /// the repeating transaction, including layout that is still settling elsewhere on screen (safe-area
    /// insets under a navigation bar, a sheet or the keyboard): sibling views then drift back and forth
    /// between two positions until some later transaction happens to replace the animation.
    @ViewBuilder
    func opacityLoop(to low: Double, halfPeriod: Double, active: Bool = true) -> some View {
        if active {
            phaseAnimator([1.0, low]) { view, opacity in
                view.opacity(opacity)
            } animation: { _ in
                .easeInOut(duration: halfPeriod)
            }
        } else {
            self
        }
    }
}
