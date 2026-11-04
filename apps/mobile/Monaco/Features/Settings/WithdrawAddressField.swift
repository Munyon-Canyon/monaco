import MonacoCore
import SwiftUI
import UIKit

enum WithdrawAddressFieldMetrics {
    static let lines = 2
    static let restingHeight: CGFloat = 56
}

enum WithdrawScan {
    @discardableResult
    static func apply(
        _ text: String, setAddress: (String) -> Void, setNetwork: (WithdrawNetwork) -> Void
    ) -> Bool {
        guard let payload = AddressPayload.parse(text) else { return false }
        if let network = payload.network { setNetwork(network) }
        setAddress(payload.address)
        return true
    }
}

struct WithdrawAddressField: View {
    @Binding var text: String
    let placeholder: String
    var isInvalid = false
    var focusRequests = 0
    var restingLine: String?
    var onScan: (() -> Void)?

    @State private var isFocused = false
    @State private var isEditing = false
    @State private var editRequests = 0
    @Environment(\.isEnabled) private var isEnabled

    private var shape: RoundedRectangle {
        RoundedRectangle(cornerRadius: MonacoTheme.Radius.compact, style: .continuous)
    }

    private var stroke: Color {
        if isInvalid { return MonacoTheme.loss }
        return isFocused ? MonacoTheme.brand : Color.clear
    }

    private var resting: String? {
        guard !isEditing, !isFocused, text == text.trimmingCharacters(in: .whitespacesAndNewlines) else {
            return nil
        }
        return restingLine
    }

    var body: some View {
        if let resting {
            restingRow(resting)
        } else {
            editingRow
        }
    }

    private func restingRow(_ line: String) -> some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Button {
                isEditing = true
                editRequests += 1
            } label: {
                Text(line)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .multilineTextAlignment(.leading)
                    .frame(maxWidth: .infinity, minHeight: MonacoButtonMetrics.hitTarget, alignment: .leading)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .disabled(!isEnabled)
            .accessibilityLabel("\(line), edit")
            .accessibilityIdentifier("withdraw-address-resting")
            actions
        }
        .padding(.leading, MonacoTheme.Space.m)
        .padding(.trailing, MonacoTheme.Space.s)
        .frame(minHeight: WithdrawAddressFieldMetrics.restingHeight)
        .background(shape.fill(MonacoTheme.surface))
        .opacity(isEnabled ? 1 : 0.5)
        .monacoPrivate()
    }

    private var editingRow: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
            ZStack(alignment: .topLeading) {
                if text.isEmpty {
                    Text(placeholder)
                        .font(MonacoTheme.Typo.data)
                        .foregroundStyle(MonacoTheme.disabledLabel)
                        .accessibilityHidden(true)
                }
                AddressTextView(
                    text: $text, isFocused: $isFocused, focusRequests: focusRequests + editRequests,
                    placeholder: placeholder,
                    isEnabled: isEnabled)
            }
            .frame(maxWidth: .infinity, alignment: .topLeading)
            actions
        }
        .padding(.leading, MonacoTheme.Space.m)
        .padding(.trailing, MonacoTheme.Space.s)
        .padding(.vertical, MonacoTheme.Space.s)
        .background(shape.fill(MonacoTheme.surfaceSunken))
        .overlay(shape.strokeBorder(stroke, lineWidth: MonacoFieldChrome.borderWidth))
        .animation(.easeOut(duration: 0.15), value: isFocused)
        .animation(.easeOut(duration: 0.15), value: isInvalid)
        .opacity(isEnabled ? 1 : 0.5)
        .onChange(of: isFocused) { _, focused in
            if !focused { isEditing = false }
        }
        .monacoPrivate()
    }

    private var actions: some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Image(systemName: "clipboard")
                .font(.system(.body))
                .foregroundStyle(MonacoTheme.tint)
                .frame(width: MonacoButtonMetrics.hitTarget, height: MonacoButtonMetrics.hitTarget)
                .accessibilityHidden(true)
                .overlay {
                    PasteButton(payloadType: String.self) { strings in
                        guard let first = strings.first else { return }
                        text = first.trimmingCharacters(in: .whitespacesAndNewlines)
                    }
                    .labelStyle(.iconOnly)
                    .buttonBorderShape(.circle)
                    .frame(minWidth: MonacoButtonMetrics.hitTarget, minHeight: MonacoButtonMetrics.hitTarget)
                    .opacity(0.015)
                    .contentShape(Rectangle())
                    .disabled(!isEnabled)
                    .accessibilityLabel("Paste")
                    .accessibilityIdentifier("withdraw-paste")
                }
            if let onScan {
                Button(action: onScan) {
                    Image(systemName: "qrcode.viewfinder")
                        .font(.system(.body))
                        .foregroundStyle(MonacoTheme.tint)
                        .frame(width: MonacoButtonMetrics.hitTarget, height: MonacoButtonMetrics.hitTarget)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Scan a QR code")
                .accessibilityIdentifier("withdraw-scan")
            }
        }
    }
}

private struct AddressTextView: UIViewRepresentable {
    @Binding var text: String
    @Binding var isFocused: Bool
    let focusRequests: Int
    let placeholder: String
    let isEnabled: Bool

    private static var font: UIFont {
        UIFontMetrics(forTextStyle: .subheadline)
            .scaledFont(for: .monospacedSystemFont(ofSize: 15, weight: .medium))
    }

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeUIView(context: Context) -> UITextView {
        let view = UITextView()
        view.delegate = context.coordinator
        view.backgroundColor = .clear
        view.isScrollEnabled = false
        view.textContainerInset = .zero
        view.textContainer.lineFragmentPadding = 0
        view.textContainer.lineBreakMode = .byCharWrapping
        view.keyboardType = .asciiCapable
        view.returnKeyType = .done
        view.autocorrectionType = .no
        view.autocapitalizationType = .none
        view.spellCheckingType = .no
        view.smartDashesType = .no
        view.smartQuotesType = .no
        view.smartInsertDeleteType = .no
        view.adjustsFontForContentSizeCategory = true
        view.font = Self.font
        view.textColor = UIColor(MonacoTheme.ink)
        view.tintColor = UIColor(MonacoTheme.ink)
        view.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        view.setContentHuggingPriority(.required, for: .vertical)
        view.accessibilityLabel = placeholder
        view.accessibilityIdentifier = "withdraw-address-field"
        return view
    }

    func updateUIView(_ view: UITextView, context: Context) {
        context.coordinator.parent = self
        view.isEditable = isEnabled
        view.accessibilityLabel = placeholder
        if view.text != text {
            view.text = text
        }
        if focusRequests != context.coordinator.focusRequests {
            context.coordinator.focusRequests = focusRequests
            DispatchQueue.main.async { view.becomeFirstResponder() }
        }
    }

    func sizeThatFits(_ proposal: ProposedViewSize, uiView: UITextView, context: Context) -> CGSize? {
        let proposed = proposal.width ?? uiView.bounds.width
        let width = proposed.isFinite && proposed > 0 ? proposed : 240
        let fitted = uiView.sizeThatFits(CGSize(width: width, height: .greatestFiniteMagnitude))
        let floor = (uiView.font?.lineHeight ?? 0) * CGFloat(WithdrawAddressFieldMetrics.lines)
        return CGSize(width: width, height: max(fitted.height, floor.rounded(.up)))
    }

    final class Coordinator: NSObject, UITextViewDelegate {
        var parent: AddressTextView
        var focusRequests = 0

        init(_ parent: AddressTextView) {
            self.parent = parent
        }

        func textViewDidChange(_ view: UITextView) {
            parent.text = view.text
        }

        func textViewDidBeginEditing(_ view: UITextView) {
            parent.isFocused = true
        }

        func textViewDidEndEditing(_ view: UITextView) {
            parent.isFocused = false
        }

        func textView(_ view: UITextView, shouldChangeTextIn range: NSRange, replacementText text: String) -> Bool {
            guard text.contains("\n") else { return true }
            view.resignFirstResponder()
            return false
        }
    }
}
