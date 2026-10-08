import SwiftUI

/// Flat paper canvas behind every screen. No gradient.
struct MonacoCanvasBackground: View {
    var body: some View {
        MonacoTheme.canvas
            .ignoresSafeArea()
    }
}

/// Canvas + ink for a tab root or pushed screen.
struct MonacoScreen<Content: View>: View {
    @ViewBuilder var content: Content

    var body: some View {
        content
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
            .foregroundStyle(MonacoTheme.ink)
    }
}

/// Capsule search field on `surfaceSunken`, 44pt tall, with a clear button while there is text.
struct MonacoSearchField: View {
    var placeholder: String
    @Binding var text: String
    var isEnabled: Bool = true

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Image(systemName: "magnifyingglass")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityHidden(true)
            TextField("", text: $text, prompt: Text(placeholder).foregroundStyle(MonacoTheme.disabledLabel))
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .tint(MonacoTheme.ink)
                .autocorrectionDisabled()
                .submitLabel(.search)
                .disabled(!isEnabled)
                .accessibilityLabel(placeholder)
                .accessibilityIdentifier("monaco-search-field")
            if !text.isEmpty, isEnabled {
                Button {
                    text = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(MonacoTheme.tertiaryText)
                        .frame(width: 44, height: 44)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Clear search")
            }
        }
        .padding(.leading, MonacoTheme.Space.m)
        .padding(.trailing, text.isEmpty ? MonacoTheme.Space.m : 0)
        .frame(minHeight: 44)
        .background(MonacoTheme.surfaceSunken, in: Capsule())
        .opacity(isEnabled ? 1 : 0.6)
    }
}
