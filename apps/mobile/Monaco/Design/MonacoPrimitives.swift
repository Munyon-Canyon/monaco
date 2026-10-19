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

enum MonacoSearchFieldMetrics {
    static let minHeight: CGFloat = 48
    static let iconSize: CGFloat = 18
}

struct MonacoSearchField: View {
    var placeholder: String
    @Binding var text: String
    var isEnabled: Bool = true

    @ScaledMetric(relativeTo: .body) private var iconSize = MonacoSearchFieldMetrics.iconSize

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Image(systemName: "magnifyingglass")
                .font(.system(size: iconSize))
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityHidden(true)
            TextField("", text: $text, prompt: Text(placeholder).foregroundStyle(MonacoTheme.secondaryText))
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
        .padding(.leading, MonacoTheme.Space.sm)
        .padding(.trailing, text.isEmpty ? MonacoTheme.Space.sm : 0)
        .frame(minHeight: MonacoSearchFieldMetrics.minHeight)
        .background(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.search, style: .continuous)
                .fill(MonacoTheme.surfaceSunken)
        )
        .opacity(isEnabled ? 1 : 0.6)
    }
}

extension View {
    func monacoSwitch() -> some View {
        toggleStyle(.switch).tint(MonacoTheme.brandFill)
    }
}

enum MonacoSelectionTickMetrics {
    static let size: CGFloat = 22
    static let glyphSize: CGFloat = 12
    static let borderWidth: CGFloat = 1
}

struct MonacoSelectionTick: View {
    let isSelected: Bool

    var body: some View {
        Circle()
            .fill(isSelected ? MonacoTheme.brandFill : Color.clear)
            .overlay {
                Circle().strokeBorder(
                    isSelected ? MonacoTheme.brandFill : MonacoTheme.border,
                    lineWidth: MonacoSelectionTickMetrics.borderWidth)
            }
            .overlay {
                if isSelected {
                    Image(systemName: "checkmark")
                        .font(.system(size: MonacoSelectionTickMetrics.glyphSize, weight: .bold))
                        .foregroundStyle(MonacoTheme.onBrand)
                }
            }
            .frame(width: MonacoSelectionTickMetrics.size, height: MonacoSelectionTickMetrics.size)
            .accessibilityElement(children: .ignore)
            .accessibilityValue(isSelected ? "Selected" : "Not selected")
    }
}
