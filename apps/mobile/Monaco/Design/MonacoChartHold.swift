import SwiftUI
import UIKit

struct MonacoChartHold: UIViewRepresentable {
    var onBegan: (CGPoint) -> Void
    var onMoved: (CGPoint) -> Void
    var onEnded: () -> Void

    func makeUIView(context: Context) -> HoldView {
        HoldView()
    }

    func updateUIView(_ view: HoldView, context: Context) {
        view.onBegan = onBegan
        view.onMoved = onMoved
        view.onEnded = onEnded
    }

    final class HoldView: UIView, UIGestureRecognizerDelegate {
        var onBegan: (CGPoint) -> Void = { _ in }
        var onMoved: (CGPoint) -> Void = { _ in }
        var onEnded: () -> Void = {}
        private var recognizer: UILongPressGestureRecognizer?
        private var isHolding = false
        private var lockedScrollViews: [UIScrollView] = []

        override func hitTest(_ point: CGPoint, with event: UIEvent?) -> UIView? { nil }

        override func didMoveToWindow() {
            super.didMoveToWindow()
            if let recognizer {
                recognizer.view?.removeGestureRecognizer(recognizer)
                self.recognizer = nil
            }
            guard let window else { return }
            let press = UILongPressGestureRecognizer(target: self, action: #selector(handle(_:)))
            press.minimumPressDuration = 0.25
            press.allowableMovement = 10
            press.cancelsTouchesInView = false
            press.delaysTouchesBegan = false
            press.delaysTouchesEnded = false
            press.delegate = self
            window.addGestureRecognizer(press)
            recognizer = press
        }

        @objc private func handle(_ press: UILongPressGestureRecognizer) {
            let point = press.location(in: self)
            switch press.state {
            case .began:
                isHolding = true
                lockPageScrolling()
                onBegan(point)
            case .changed:
                onMoved(point)
            case .ended, .cancelled, .failed:
                guard isHolding else { return }
                isHolding = false
                unlockPageScrolling()
                onEnded()
            default:
                break
            }
        }

        private func lockPageScrolling() {
            var ancestor = superview
            while let view = ancestor {
                if let scroll = view as? UIScrollView, scroll.panGestureRecognizer.isEnabled {
                    scroll.panGestureRecognizer.isEnabled = false
                    lockedScrollViews.append(scroll)
                }
                ancestor = view.superview
            }
        }

        private func unlockPageScrolling() {
            for scroll in lockedScrollViews { scroll.panGestureRecognizer.isEnabled = true }
            lockedScrollViews = []
        }

        override func willMove(toWindow newWindow: UIWindow?) {
            if newWindow == nil { unlockPageScrolling() }
            super.willMove(toWindow: newWindow)
        }

        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldReceive touch: UITouch) -> Bool {
            bounds.contains(touch.location(in: self))
        }

        func gestureRecognizer(
            _ gestureRecognizer: UIGestureRecognizer,
            shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer
        ) -> Bool {
            true
        }
    }
}
