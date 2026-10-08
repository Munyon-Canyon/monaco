import MonacoAPI
import Observation
import SwiftUI

@Observable
@MainActor
final class ToastCenter {
    var current: MonacoToast?

    func show(_ error: APIError) {
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
    func monacoToastCenter(
        _ center: ToastCenter, placement: MonacoToastPlacement = .screenBottom, isEnabled: Bool = true
    ) -> some View {
        modifier(ToastCenterHost(center: center, placement: placement, isEnabled: isEnabled))
    }
}

private struct ToastCenterHost: ViewModifier {
    @Bindable var center: ToastCenter
    let placement: MonacoToastPlacement
    let isEnabled: Bool

    func body(content: Content) -> some View {
        content.monacoToast(
            Binding(
                get: { isEnabled ? center.current : nil },
                set: { center.current = $0 }
            ),
            placement: placement
        )
    }
}
