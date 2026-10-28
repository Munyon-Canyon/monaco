import CoreImage
import CoreImage.CIFilterBuiltins
import SwiftUI

struct DepositQRCode: View {
    static let side: CGFloat = 200
    static let accessibilityLabelText = "QR code of your deposit address"

    let address: String
    var showsSolanaMark = true
    let onCopy: (String) -> Void

    static func image(for address: String) -> CIImage? {
        QRImage.image(for: address)
    }

    var body: some View {
        ZStack {
            if let cgImage = QRImage.cgImage(for: address) {
                Image(decorative: cgImage, scale: 1)
                    .interpolation(.none)
                    .resizable()
                    .scaledToFit()
                    .padding(MonacoTheme.Space.m)
            }
            if showsSolanaMark {
                Image("SolanaMark")
                    .resizable()
                    .scaledToFit()
                    .frame(width: 24)
                    .padding(MonacoTheme.Space.s)
                    .background(Color.white, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip))
                    .accessibilityHidden(true)
            }
        }
        .frame(width: Self.side, height: Self.side)
        .background(Color.white, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.accessibilityLabelText)
        .accessibilityIdentifier("deposit-address-qr")
        .contextMenu {
            Button("Copy address") {
                onCopy(address)
            }
        }
    }
}

enum QRImage {
    static func image(for text: String) -> CIImage? {
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(text.utf8)
        filter.correctionLevel = "M"
        return filter.outputImage
    }

    static func cgImage(for text: String) -> CGImage? {
        image(for: text).flatMap { context.createCGImage($0, from: $0.extent) }
    }

    private static let context = CIContext()
}
