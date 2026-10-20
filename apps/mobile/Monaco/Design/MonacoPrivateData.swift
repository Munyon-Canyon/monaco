import PostHog
import SwiftUI

extension View {
    func monacoPrivate(_ isPrivate: Bool = true) -> some View {
        postHogMask(isPrivate)
    }

    func monacoPrivateFigure() -> some View {
        monacoPrivate().accessibilityElement(children: .combine)
    }

    func pausesSessionReplay(_ isActive: Bool = true) -> some View {
        modifier(PausesSessionReplay(isActive: isActive))
    }
}

private struct PausesSessionReplay: ViewModifier {
    let isActive: Bool
    @State private var paused = false

    func body(content: Content) -> some View {
        content
            .onChange(of: isActive, initial: true) { _, active in apply(active) }
            .onDisappear { apply(false) }
    }

    private func apply(_ active: Bool) {
        guard active != paused else { return }
        paused = active
        if active { SessionReplayGate.pause() } else { SessionReplayGate.resume() }
    }
}
