import SwiftUI

enum MonacoUnderlineTabsMetrics {
    static let minHeight: CGFloat = 44
    static let barHeight: CGFloat = 2
    static let labelSpacing: CGFloat = 24
    static let countGap: CGFloat = 4
}

struct MonacoUnderlineTabs<T: Hashable>: View {
    private let options: [T]
    @Binding private var selection: T
    private let count: ((T) -> Int?)?
    private let identifier: ((T) -> String)?
    private let label: (T) -> String

    @Namespace private var bar
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.displayScale) private var displayScale

    init(
        _ options: [T], selection: Binding<T>, count: ((T) -> Int?)? = nil,
        identifier: ((T) -> String)? = nil, label: @escaping (T) -> String
    ) {
        self.options = options
        _selection = selection
        self.count = count
        self.identifier = identifier
        self.label = label
    }

    var body: some View {
        ViewThatFits(in: .horizontal) {
            row.frame(maxWidth: .infinity, alignment: .leading)
            ScrollView(.horizontal, showsIndicators: false) { row }
        }
        .frame(minHeight: MonacoUnderlineTabsMetrics.minHeight)
        .overlay(alignment: .bottom) {
            Rectangle().fill(MonacoTheme.hairline).frame(height: 1 / displayScale)
        }
    }

    private var row: some View {
        HStack(spacing: MonacoUnderlineTabsMetrics.labelSpacing) {
            ForEach(options, id: \.self) { option in
                tab(option)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private func tab(_ option: T) -> some View {
        let isSelected = option == selection
        let number = count?(option)
        return Button {
            guard option != selection else { return }
            Haptics.selection()
            withAnimation(reduceMotion ? nil : .spring(response: 0.3, dampingFraction: 0.85)) {
                selection = option
            }
        } label: {
            HStack(spacing: MonacoUnderlineTabsMetrics.countGap) {
                Text(label(option))
                    .foregroundStyle(isSelected ? MonacoTheme.primaryText : MonacoTheme.secondaryText)
                if let number {
                    Text("\(number)").foregroundStyle(MonacoTheme.secondaryText)
                }
            }
            .font(MonacoTheme.Typo.section)
            .lineLimit(1)
            .fixedSize(horizontal: true, vertical: false)
            .frame(minHeight: MonacoUnderlineTabsMetrics.minHeight)
            .overlay(alignment: .bottom) {
                if isSelected {
                    Rectangle()
                        .fill(MonacoTheme.primaryText)
                        .frame(height: MonacoUnderlineTabsMetrics.barHeight)
                        .matchedGeometryEffect(id: "bar", in: bar)
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(number.map { "\(label(option)), \($0)" } ?? label(option))
        .accessibilityAddTraits(isSelected ? [.isSelected] : [])
        .tabIdentifier(identifier?(option))
    }
}

extension View {
    func monacoUnderlineTabsPinned() -> some View {
        background(MonacoTheme.canvas)
    }

    @ViewBuilder
    fileprivate func tabIdentifier(_ identifier: String?) -> some View {
        if let identifier {
            accessibilityIdentifier(identifier)
        } else {
            self
        }
    }
}
