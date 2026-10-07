import CoreImage
import CoreImage.CIFilterBuiltins
import SwiftUI

struct DepositQRCode: View {
    static let side: CGFloat = 200
    static let accessibilityLabelText = "QR code of your deposit address"

    let address: String
    let onCopy: (String) -> Void

    static func image(for address: String) -> CIImage? {
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(address.utf8)
        filter.correctionLevel = "M"
        return filter.outputImage
    }

    var body: some View {
        ZStack {
            if let image = Self.image(for: address), let cgImage = Self.context.createCGImage(image, from: image.extent)
            {
                Image(decorative: cgImage, scale: 1)
                    .interpolation(.none)
                    .resizable()
                    .scaledToFit()
                    .padding(MonacoTheme.Space.m)
            }
            Image("SolanaMark")
                .resizable()
                .scaledToFit()
                .frame(width: 24)
                .padding(MonacoTheme.Space.s)
                .background(Color.white, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip))
                .accessibilityHidden(true)
        }
        .frame(width: Self.side, height: Self.side)
        .background(Color.white, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.accessibilityLabelText)
        .accessibilityIdentifier("deposit-address-qr")
        .contextMenu {
            Button("Copy address") {
                onCopy(address)
            }
        }
    }

    private static let context = CIContext()
}
