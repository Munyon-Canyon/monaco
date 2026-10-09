import SwiftUI

/// Capsule track on `surfaceSunken` with an ink thumb that slides between options:
/// the selected label in canvas on the thumb, the others muted on the track.
/// Replaces `Picker(.segmented)` and its global appearance hack.
struct MonacoSegmented<T: Hashable>: View {
    private let options: [T]
    @Binding private var selection: T
    private let label: (T) -> String

    @Namespace private var thumb
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init(_ options: [T], selection: Binding<T>, label: @escaping (T) -> String) {
        self.options = options
        _selection = selection
        self.label = label
    }

    var body: some View {
        HStack(spacing: 0) {
            ForEach(options, id: \.self) { option in
                let isSelected = option == selection
                Button {
                    guard option != selection else { return }
                    Haptics.selection()
                    withAnimation(reduceMotion ? nil : .spring(response: 0.3, dampingFraction: 0.85)) {
                        selection = option
                    }
                } label: {
                    Text(label(option))
                        .font(MonacoTheme.Typo.calloutStrong)
                        .lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
                        .minimumScaleFactor(0.8)
                        .multilineTextAlignment(.center)
                        .foregroundStyle(isSelected ? MonacoTheme.onBrand : MonacoTheme.ink)
                        .padding(.horizontal, MonacoTheme.Space.m)
                        .frame(maxWidth: .infinity, minHeight: 44)
                        .background {
                            if isSelected {
                                Capsule()
                                    .fill(MonacoTheme.brandFill)
                                    .padding(MonacoTheme.Space.xs)
                                    .matchedGeometryEffect(id: "thumb", in: thumb)
                            }
                        }
                        .contentShape(Capsule())
                }
                .buttonStyle(.plain)
                .accessibilityAddTraits(isSelected ? [.isSelected] : [])
            }
        }
        .frame(minHeight: 44)
        .background(Capsule().fill(MonacoTheme.surfaceSunken))
    }
}
