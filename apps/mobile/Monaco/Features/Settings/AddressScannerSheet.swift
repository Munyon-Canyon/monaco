import MonacoCore
import SwiftUI
import Vision
import VisionKit

enum AddressScannerCopy {
    static let caption = "Point at the QR code of the address you're sending to."
    static let notAnAddress = "That code isn't an address."
    static let unavailable = "Camera isn't available. Paste the address instead."
}

enum AddressScanner {
    static var isSupported: Bool { DataScannerViewController.isSupported }
    static var isAvailable: Bool { DataScannerViewController.isSupported && DataScannerViewController.isAvailable }
}

struct AddressScannerSheet: View {
    let onResult: (String) -> Void

    @Environment(\.dismiss) private var dismiss
    @Environment(ToastCenter.self) private var toasts
    @State private var rejected: Set<String> = []

    var body: some View {
        ZStack(alignment: .top) {
            AddressScannerView { text in handle(text) }
                .ignoresSafeArea()
            HStack {
                Spacer()
                Button {
                    dismiss()
                } label: {
                    Image(systemName: "xmark")
                        .font(.system(.body, weight: .semibold))
                        .foregroundStyle(MonacoTheme.primaryText)
                        .frame(width: MonacoButtonMetrics.hitTarget, height: MonacoButtonMetrics.hitTarget)
                        .background(Circle().fill(.regularMaterial))
                        .contentShape(Circle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Close")
                .accessibilityIdentifier("address-scanner-close")
            }
            .padding(MonacoTheme.Space.m)
        }
        .overlay(alignment: .bottom) {
            Text(AddressScannerCopy.caption)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.primaryText)
                .multilineTextAlignment(.center)
                .padding(MonacoTheme.Space.m)
                .background(RoundedRectangle(cornerRadius: MonacoTheme.Radius.field).fill(.regularMaterial))
                .padding(MonacoTheme.Space.m)
        }
        .accessibilityIdentifier("address-scanner")
    }

    private func handle(_ text: String) {
        if AddressPayload.parse(text) != nil {
            Haptics.success()
            onResult(text)
            dismiss()
        } else if rejected.insert(text).inserted {
            Haptics.warning()
            toasts.current = MonacoToast(message: AddressScannerCopy.notAnAddress)
        }
    }
}

private struct AddressScannerView: UIViewControllerRepresentable {
    let onPayload: (String) -> Void

    func makeCoordinator() -> Coordinator { Coordinator(onPayload: onPayload) }

    func makeUIViewController(context: Context) -> DataScannerViewController {
        let controller = DataScannerViewController(
            recognizedDataTypes: [.barcode(symbologies: [.qr])], qualityLevel: .balanced,
            isHighlightingEnabled: true)
        controller.delegate = context.coordinator
        return controller
    }

    func updateUIViewController(_ controller: DataScannerViewController, context: Context) {
        context.coordinator.onPayload = onPayload
        guard !controller.isScanning else { return }
        DispatchQueue.main.async { try? controller.startScanning() }
    }

    @MainActor
    final class Coordinator: NSObject, DataScannerViewControllerDelegate {
        var onPayload: (String) -> Void

        init(onPayload: @escaping (String) -> Void) {
            self.onPayload = onPayload
        }

        func dataScanner(
            _ scanner: DataScannerViewController, didAdd items: [RecognizedItem], allItems: [RecognizedItem]
        ) {
            for item in items { report(item) }
        }

        func dataScanner(_ scanner: DataScannerViewController, didTapOn item: RecognizedItem) {
            report(item)
        }

        private func report(_ item: RecognizedItem) {
            guard case .barcode(let barcode) = item, let text = barcode.payloadStringValue else { return }
            onPayload(text)
        }
    }
}
