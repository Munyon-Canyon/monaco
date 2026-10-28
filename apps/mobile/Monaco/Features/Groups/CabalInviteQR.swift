import SwiftUI

struct CabalInviteQR: View {
    let link: URL
    let cabalName: String

    static func accessibilityLabelText(cabalName: String) -> String {
        "QR code of the invite link to \(cabalName)"
    }

    static func caption(cabalName: String) -> String { "Scan to join \(cabalName)" }

    var body: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            ZStack {
                if let image = QRImage.cgImage(for: link.absoluteString) {
                    Image(decorative: image, scale: 1)
                        .interpolation(.none)
                        .resizable()
                        .scaledToFit()
                        .padding(MonacoTheme.Space.m)
                }
            }
            .frame(width: CabalInviteMetrics.qrTile, height: CabalInviteMetrics.qrTile)
            .background(Color.white, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous))
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Self.accessibilityLabelText(cabalName: cabalName))
            .accessibilityIdentifier("cabal-invite-qr")
            Text(Self.caption(cabalName: cabalName))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, MonacoTheme.Space.m)
        .background(MonacoTheme.surfaceSunken)
        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
    }
}

struct CabalInviteFlip<Front: View, Back: View>: View, Animatable {
    var angle: Double
    let front: Front
    let back: Back

    var animatableData: Double {
        get { angle }
        set { angle = newValue }
    }

    private var showsBack: Bool { angle >= 90 }

    var body: some View {
        ZStack {
            front
                .opacity(showsBack ? 0 : 1)
                .accessibilityHidden(showsBack)
                .allowsHitTesting(!showsBack)
            back
                .rotation3DEffect(.degrees(180), axis: (x: 0, y: 1, z: 0))
                .opacity(showsBack ? 1 : 0)
                .accessibilityHidden(!showsBack)
                .allowsHitTesting(showsBack)
        }
        .rotation3DEffect(.degrees(angle), axis: (x: 0, y: 1, z: 0))
    }
}
