import MonacoAPI
import Observation
import SwiftUI

@Observable
@MainActor
final class ToastCenter {
    static let errorBurst: TimeInterval = 1

    private var shown: MonacoToast?
    @ObservationIgnored private var shownAt = Date.distantPast
    private let now: () -> Date

    init(now: @escaping () -> Date = Date.init) {
        self.now = now
    }

    var current: MonacoToast? {
        get { shown }
        set {
            if let newValue, let shown, !newValue.isSuccess, !shown.isSuccess,
                newValue.message == shown.message,
                now().timeIntervalSince(shownAt) < Self.errorBurst
            {
                return
            }
            shown = newValue
            if newValue != nil { shownAt = now() }
        }
    }

    func show(_ error: APIError) {
        if case .cancelled = error { return }
        current = MonacoToast(message: ToastCopy.message(for: error))
    }

    func show(success: String, link: MonacoToastLink? = nil, action: MonacoToastAction? = nil) {
        current = MonacoToast(message: success, isSuccess: true, link: link, action: action)
    }
}

nonisolated struct TabToastHostKey: PreferenceKey {
    static let defaultValue = false

    static func reduce(value: inout Bool, nextValue: () -> Bool) {
        value = value || nextValue()
    }
}

extension View {
    func monacoToastCenter(_ center: ToastCenter, isEnabled: Bool = true) -> some View {
        modifier(ToastCenterHost(center: center, isEnabled: isEnabled))
    }
}

private struct ToastCenterHost: ViewModifier {
    @Bindable var center: ToastCenter
    let isEnabled: Bool

    func body(content: Content) -> some View {
        content.monacoToast(
            Binding(
                get: { isEnabled ? center.current : nil },
                set: { center.current = $0 }
            )
        )
    }
}
