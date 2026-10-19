import SwiftUI

enum MonacoSegmentedMetrics {
    static let inset: CGFloat = 3
    static let spacing: CGFloat = 2
    static let minHeight: CGFloat = 42
    static let periodsMinHeight: CGFloat = 44
    static let hitOutset: CGFloat = 4
    static let borderWidth: CGFloat = 1
}

struct MonacoSegmentedLayout: Equatable {
    let minHeight: CGFloat

    static let standard = MonacoSegmentedLayout(minHeight: MonacoSegmentedMetrics.minHeight)
    static let periods = MonacoSegmentedLayout(minHeight: MonacoSegmentedMetrics.periodsMinHeight)
}

struct MonacoSegmented<T: Hashable>: View {
    private let options: [T]
    @Binding private var selection: T
    private let layout: MonacoSegmentedLayout
    private let label: (T) -> String
    private let accessibilityName: ((T) -> String)?
    private let identifier: ((T) -> String)?

    @Namespace private var thumb
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init(
        _ options: [T], selection: Binding<T>, layout: MonacoSegmentedLayout = .standard,
        accessibilityName: ((T) -> String)? = nil, identifier: ((T) -> String)? = nil,
        label: @escaping (T) -> String
    ) {
        self.options = options
        _selection = selection
        self.layout = layout
        self.label = label
        self.accessibilityName = accessibilityName
        self.identifier = identifier
    }

    var body: some View {
        HStack(spacing: MonacoSegmentedMetrics.spacing) {
            ForEach(options, id: \.self) { option in
                segment(option)
            }
        }
        .padding(MonacoSegmentedMetrics.inset)
        .frame(maxWidth: .infinity, minHeight: layout.minHeight)
        .background(Capsule().fill(MonacoTheme.surfaceSunken))
        .overlay {
            Capsule().strokeBorder(MonacoTheme.border, lineWidth: MonacoSegmentedMetrics.borderWidth)
        }
    }

    private func segment(_ option: T) -> some View {
        let isSelected = option == selection
        return Button {
            guard option != selection else { return }
            Haptics.selection()
            withAnimation(reduceMotion ? nil : .spring(response: 0.3, dampingFraction: 0.85)) {
                selection = option
            }
        } label: {
            Text(label(option))
                .font(isSelected ? MonacoTheme.Typo.captionStrong : MonacoTheme.Typo.caption)
                .lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
                .minimumScaleFactor(0.8)
                .multilineTextAlignment(.center)
                .foregroundStyle(isSelected ? MonacoTheme.onBrand : MonacoTheme.secondaryText)
                .padding(.horizontal, MonacoTheme.Space.sm)
                .padding(.vertical, MonacoTheme.Space.xs)
                .frame(maxWidth: .infinity, minHeight: layout.minHeight - 2 * MonacoSegmentedMetrics.inset)
                .background {
                    if isSelected {
                        Capsule()
                            .fill(MonacoTheme.brandFill)
                            .matchedGeometryEffect(id: "thumb", in: thumb)
                    }
                }
                .contentShape(Rectangle().inset(by: -MonacoSegmentedMetrics.hitOutset))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(accessibilityName?(option) ?? label(option))
        .accessibilityAddTraits(isSelected ? [.isSelected] : [])
        .segmentIdentifier(identifier?(option))
    }
}

extension View {
    @ViewBuilder
    fileprivate func segmentIdentifier(_ identifier: String?) -> some View {
        if let identifier {
            accessibilityIdentifier(identifier)
        } else {
            self
        }
    }
}
