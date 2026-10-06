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

extension View {
    func monacoToastCenter(_ center: ToastCenter, placement: MonacoToastPlacement = .screenBottom) -> some View {
        modifier(ToastCenterHost(center: center, placement: placement))
    }
}

private struct ToastCenterHost: ViewModifier {
    @Bindable var center: ToastCenter
    let placement: MonacoToastPlacement

    func body(content: Content) -> some View {
        content.monacoToast(
            Binding(
                get: { center.current },
                set: { center.current = $0 }
            ),
            placement: placement
        )
    }
}
