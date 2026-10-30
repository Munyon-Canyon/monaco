import SwiftUI

struct ChainMark: View {
    let network: String
    var size: CGFloat = 40

    static func imageName(for network: String) -> String? {
        switch network {
        case "solana": "SolanaMark"
        case "base": "BaseMark"
        case "robinhood": "RobinhoodChainMark"
        default: nil
        }
    }

    var body: some View {
        if let name = Self.imageName(for: network) {
            Image(name)
                .resizable()
                .scaledToFit()
                .frame(width: size * 0.7, height: size * 0.7)
                .frame(width: size, height: size)
                .accessibilityHidden(true)
        } else {
            SunkenGlyphMark(systemImage: "point.3.connected.trianglepath.dotted", size: size)
        }
    }
}
