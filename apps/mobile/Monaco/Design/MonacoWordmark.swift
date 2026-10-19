import SwiftUI

struct MonacoWordmark: View {
    enum Style {
        case toolbar, signIn

        fileprivate var baseSize: CGFloat {
            switch self {
            case .toolbar: return 28
            case .signIn: return 36
            }
        }

        fileprivate var maxSize: CGFloat {
            switch self {
            case .toolbar: return 34
            case .signIn: return 44
            }
        }
    }

    let style: Style
    var isTitle = false

    @ScaledMetric(relativeTo: .title) private var scaledSize: CGFloat = 28

    init(_ style: Style, isTitle: Bool = false) {
        self.style = style
        self.isTitle = isTitle
        _scaledSize = ScaledMetric(wrappedValue: style.baseSize, relativeTo: .title)
    }

    var body: some View {
        Text("monaco")
            .font(.system(size: min(scaledSize, style.maxSize), weight: .bold))
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(1)
            .accessibilityLabel("Monaco")
            .accessibilityAddTraits(isTitle ? .isHeader : [])
    }
}
