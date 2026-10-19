import SwiftUI

extension View {
    /// `MonacoTextField`'s anatomy, drawn by the same `MonacoFieldChrome`, for the sign-in fields:
    /// they need an external focus binding (the form moves focus from the address to the code)
    /// and identifiers the UI tests sign in by, which `MonacoTextField` does not expose.
    func authTextFieldStyle(isFocused: Bool, isInvalid: Bool = false, focus: @escaping () -> Void) -> some View {
        font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.ink)
            .tint(MonacoTheme.ink)
            .monacoFieldChrome(isFocused: isFocused, isInvalid: isInvalid, focus: focus)
    }

    func authScreenBackground() -> some View {
        monacoCanvas()
    }

    func authHeadline() -> some View {
        font(MonacoTheme.Typo.display)
            .foregroundStyle(MonacoTheme.ink)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: 290, alignment: .leading)
            .accessibilityAddTraits(.isHeader)
    }

    func authStepTransition(reduceMotion: Bool) -> some View {
        transition(reduceMotion ? .identity : .move(edge: .trailing).combined(with: .opacity))
    }
}
