import MonacoCore
import SwiftUI

struct ShareCardFace: View {
    enum Shape {
        case circle
        case tile
    }

    let image: UIImage?
    let name: String
    let seed: String
    let size: CGFloat
    let shape: Shape

    static func tint(seed: String) -> MonacoTheme.CabalTint {
        .forGroupId(seed)
    }

    var body: some View {
        face
            .frame(width: size, height: size)
            .clipShape(clip)
    }

    @ViewBuilder
    private var face: some View {
        if let image {
            Image(uiImage: image)
                .resizable()
                .scaledToFill()
        } else {
            Rectangle()
                .fill(Self.tint(seed: seed).fill)
                .overlay {
                    Text(CabalMark.initials(for: name))
                        .font(.system(size: size * 0.4, weight: .semibold))
                        .foregroundStyle(Color.white)
                        .lineLimit(1)
                        .minimumScaleFactor(0.5)
                }
        }
    }

    private var clip: AnyShape {
        switch shape {
        case .circle: AnyShape(Circle())
        case .tile: AnyShape(RoundedRectangle(cornerRadius: size * 0.24, style: .continuous))
        }
    }
}
