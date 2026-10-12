import SwiftUI

struct MonacoChartStyleToggle: View {
    @Binding var style: MonacoChartStyle
    @Namespace private var thumb
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        HStack(spacing: 0) {
            ForEach(MonacoChartStyle.allCases, id: \.self) { option in
                let isSelected = option == style
                Button {
                    guard !isSelected else { return }
                    Haptics.selection()
                    withAnimation(reduceMotion ? nil : .spring(response: 0.3, dampingFraction: 0.85)) {
                        style = option
                    }
                } label: {
                    Image(systemName: option.symbol)
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(isSelected ? MonacoTheme.onBrand : MonacoTheme.ink)
                        .frame(width: 44, height: 44)
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
                .accessibilityLabel(option.accessibilityName)
                .accessibilityAddTraits(isSelected ? [.isSelected] : [])
                .accessibilityIdentifier("asset-chart-style-\(option.rawValue)")
            }
        }
        .background(Capsule().fill(MonacoTheme.surfaceSunken))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-chart-style")
    }
}
