import SwiftUI

enum MonacoMenuSelectMetrics {
    static let minHeight: CGFloat = 44
    static let minWidth: CGFloat = 85
    static let chevronSize: CGFloat = 13
    static let borderWidth: CGFloat = 1
}

struct MonacoMenuSelect<Option: Hashable>: View {
    private let title: String
    @Binding private var selection: Option
    private let options: [Option]
    private let label: (Option) -> String
    private var expands = false

    @ScaledMetric(relativeTo: .footnote) private var chevronSize = MonacoMenuSelectMetrics.chevronSize

    init(
        title: String, selection: Binding<Option>, options: [Option], label: @escaping (Option) -> String
    ) {
        self.title = title
        _selection = selection
        self.options = options
        self.label = label
    }

    func fillsWidth(_ enabled: Bool = true) -> MonacoMenuSelect {
        var copy = self
        copy.expands = enabled
        return copy
    }

    var body: some View {
        Menu {
            Picker(title, selection: $selection) {
                ForEach(options, id: \.self) { option in
                    Text(label(option)).tag(option)
                }
            }
            .pickerStyle(.inline)
        } label: {
            HStack(spacing: MonacoTheme.Space.s) {
                Text(label(selection))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                if expands {
                    Spacer(minLength: 0)
                }
                Image(systemName: "chevron.down")
                    .font(.system(size: chevronSize, weight: .semibold))
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
            }
            .padding(.horizontal, MonacoTheme.Space.m)
            .frame(
                minWidth: MonacoMenuSelectMetrics.minWidth,
                maxWidth: expands ? .infinity : nil,
                minHeight: MonacoMenuSelectMetrics.minHeight
            )
            .background(Capsule().fill(MonacoTheme.surface))
            .overlay(Capsule().strokeBorder(MonacoTheme.border, lineWidth: MonacoMenuSelectMetrics.borderWidth))
            .contentShape(Capsule())
        }
        .accessibilityLabel("\(title), \(label(selection))")
        .accessibilityAddTraits(.isButton)
    }
}
