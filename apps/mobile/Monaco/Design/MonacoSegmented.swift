import SwiftUI

enum MonacoSegmentedMetrics {
    static let inset: CGFloat = 3
    static let spacing: CGFloat = 2
    static let minHeight: CGFloat = 42
    static let hitOutset: CGFloat = 4
    static let thumbRadius: CGFloat = 6
    static let stackedOptionLimit = 3
}

struct MonacoSegmentedLayout: Equatable {
    let minHeight: CGFloat

    static let standard = MonacoSegmentedLayout(minHeight: MonacoSegmentedMetrics.minHeight)
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
        segments
            .padding(MonacoSegmentedMetrics.inset)
            .frame(maxWidth: .infinity, minHeight: layout.minHeight)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.small, style: .continuous)
                    .fill(MonacoTheme.surfaceSunken))
    }

    @ViewBuilder private var segments: some View {
        if dynamicTypeSize >= .xxLarge {
            ViewThatFits(in: .horizontal) {
                HStack(spacing: MonacoSegmentedMetrics.spacing) { segmentViews(keepsWidth: true) }
                if options.count > MonacoSegmentedMetrics.stackedOptionLimit {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: MonacoSegmentedMetrics.spacing) { segmentViews(keepsWidth: true) }
                    }
                } else {
                    VStack(spacing: MonacoSegmentedMetrics.spacing) { segmentViews(keepsWidth: false) }
                }
            }
        } else {
            HStack(spacing: MonacoSegmentedMetrics.spacing) { segmentViews(keepsWidth: false) }
        }
    }

    private func segmentViews(keepsWidth: Bool) -> some View {
        ForEach(options, id: \.self) { option in
            segment(option, keepsWidth: keepsWidth)
        }
    }

    private func segment(_ option: T, keepsWidth: Bool) -> some View {
        let isSelected = option == selection
        return Button {
            guard option != selection else { return }
            Haptics.selection()
            withAnimation(reduceMotion ? nil : .spring(response: 0.3, dampingFraction: 0.85)) {
                selection = option
            }
        } label: {
            Text(label(option))
                .font(MonacoTheme.Typo.subheadStrong)
                .lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
                .minimumScaleFactor(0.8)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: keepsWidth, vertical: false)
                .foregroundStyle(isSelected ? MonacoTheme.primaryText : MonacoTheme.secondaryText)
                .padding(.horizontal, MonacoTheme.Space.sm)
                .padding(.vertical, MonacoTheme.Space.xs)
                .frame(maxWidth: .infinity, minHeight: layout.minHeight - 2 * MonacoSegmentedMetrics.inset)
                .background {
                    if isSelected {
                        RoundedRectangle(cornerRadius: MonacoSegmentedMetrics.thumbRadius, style: .continuous)
                            .fill(MonacoTheme.surface)
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
