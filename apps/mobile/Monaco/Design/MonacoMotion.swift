import SwiftUI

enum MonacoMotion {
    static let state = Animation.easeOut(duration: 0.2)
    static let roll = Animation.snappy(duration: 0.25)
    static let settle = Spring(response: 0.45, dampingRatio: 0.72)
    static let settleScale: CGFloat = 1.03
    static let settleLift: TimeInterval = 0.08
    static let stamp = Animation.spring(response: 0.38, dampingFraction: 0.6)
    static let stampScale: CGFloat = 0.6
    static let drawOn = Animation.easeOut(duration: 0.45)
}

extension View {
    func monacoSettle(on value: some Equatable & Sendable) -> some View {
        modifier(MonacoSettle(trigger: value))
    }

    func monacoStamp() -> some View {
        modifier(MonacoStamp())
    }

    func monacoDrawOn() -> some View {
        modifier(MonacoDrawOn())
    }
}

private struct MonacoSettle<Trigger: Equatable & Sendable>: ViewModifier {
    let trigger: Trigger
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        if reduceMotion {
            content
        } else {
            content.keyframeAnimator(initialValue: CGFloat(1), trigger: trigger) { view, scale in
                view.scaleEffect(scale)
            } keyframes: { _ in
                CubicKeyframe(MonacoMotion.settleScale, duration: MonacoMotion.settleLift)
                SpringKeyframe(1, spring: MonacoMotion.settle)
            }
        }
    }
}

private struct MonacoStamp: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var landed = false

    func body(content: Content) -> some View {
        content
            .scaleEffect(landed || reduceMotion ? 1 : MonacoMotion.stampScale)
            .opacity(landed ? 1 : 0)
            .onAppear {
                withAnimation(reduceMotion ? MonacoMotion.state : MonacoMotion.stamp) { landed = true }
            }
    }
}

private struct MonacoDrawOn: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var drawn = false

    func body(content: Content) -> some View {
        content
            .scaleEffect(x: drawn || reduceMotion ? 1 : 0, y: 1, anchor: .leading)
            .onAppear {
                if reduceMotion {
                    drawn = true
                } else {
                    withAnimation(MonacoMotion.drawOn) { drawn = true }
                }
            }
    }
}
