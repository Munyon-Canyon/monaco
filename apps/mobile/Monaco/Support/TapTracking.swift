import MonacoAnalytics
import SwiftUI
import UIKit

@MainActor
final class TapObserver: NSObject, UIGestureRecognizerDelegate {
    private let analytics: Analytics
    private let origin = ContinuousClock.now

    init(analytics: Analytics) {
        self.analytics = analytics
    }

    func install(in window: UIWindow) {
        let recognizer = UITapGestureRecognizer(target: self, action: #selector(tapped(_:)))
        recognizer.cancelsTouchesInView = false
        recognizer.delegate = self
        window.addGestureRecognizer(recognizer)
    }

    nonisolated func gestureRecognizer(
        _ gestureRecognizer: UIGestureRecognizer,
        shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer
    ) -> Bool {
        true
    }

    @objc private func tapped(_ recognizer: UITapGestureRecognizer) {
        guard let window = recognizer.view as? UIWindow else { return }
        let point = recognizer.location(in: window)
        guard let target = TapTarget.resolve(at: point, in: window) else { return }
        analytics.tap(
            controlID: target.identifier,
            screen: analytics.currentScreen,
            interactive: target.isInteractive,
            at: origin.duration(to: .now)
        )
    }
}

struct TapTarget {
    let identifier: String
    let isInteractive: Bool

    @MainActor
    static func resolve(at point: CGPoint, in window: UIWindow) -> TapTarget? {
        guard let hit = window.hitTest(point, with: nil) else { return nil }
        let screenPoint = window.convert(point, to: nil)
        if let element = accessibilityElement(containing: screenPoint, in: hit),
            let identifier = (element as? UIAccessibilityIdentification)?.accessibilityIdentifier, !identifier.isEmpty
        {
            return TapTarget(
                identifier: identifier,
                isInteractive: isInteractive(traits: element.accessibilityTraits) || hasControlAncestor(hit)
            )
        }
        var view: UIView? = hit
        while let current = view {
            if let identifier = current.accessibilityIdentifier, !identifier.isEmpty {
                return TapTarget(identifier: identifier, isInteractive: isInteractive(view: current))
            }
            view = current.superview
        }
        return nil
    }

    private static func isInteractive(view: UIView) -> Bool {
        view is UIControl || isInteractive(traits: view.accessibilityTraits)
            || !(view.gestureRecognizers ?? []).isEmpty
    }

    private static func isInteractive(traits: UIAccessibilityTraits) -> Bool {
        traits.contains(.button) || traits.contains(.link) || traits.contains(.adjustable)
    }

    private static func hasControlAncestor(_ view: UIView) -> Bool {
        var current: UIView? = view
        while let candidate = current {
            if candidate is UIControl { return true }
            current = candidate.superview
        }
        return false
    }

    private static func accessibilityElement(containing point: CGPoint, in view: UIView) -> NSObject? {
        var best: (element: NSObject, area: CGFloat)?
        var stack: [Any] = [view]
        var visited = 0
        while let next = stack.popLast(), visited < 400 {
            visited += 1
            guard let object = next as? NSObject else { continue }
            let frame = object.accessibilityFrame
            if object.isAccessibilityElement, frame.contains(point) {
                let area = frame.width * frame.height
                if best == nil || area < best?.area ?? .infinity { best = (object, area) }
            }
            stack.append(contentsOf: object.accessibilityElements ?? [])
            if let view = object as? UIView { stack.append(contentsOf: view.subviews) }
        }
        return best?.element
    }
}

struct TapTrackingInstaller: UIViewRepresentable {
    let analytics: Analytics

    func makeCoordinator() -> Coordinator {
        Coordinator(observer: TapObserver(analytics: analytics))
    }

    func makeUIView(context: Context) -> InstallerView {
        let view = InstallerView()
        view.isUserInteractionEnabled = false
        view.observer = context.coordinator.observer
        return view
    }

    func updateUIView(_ uiView: InstallerView, context: Context) {}

    @MainActor
    final class Coordinator {
        let observer: TapObserver

        init(observer: TapObserver) {
            self.observer = observer
        }
    }

    final class InstallerView: UIView {
        var observer: TapObserver?
        private var installed = false

        override func didMoveToWindow() {
            super.didMoveToWindow()
            guard !installed, let window, let observer else { return }
            installed = true
            observer.install(in: window)
        }
    }
}
