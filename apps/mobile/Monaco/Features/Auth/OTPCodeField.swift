import SwiftUI

enum OTPBoxMetrics {
    static let height: CGFloat = 56
    static let spacing: CGFloat = 8
    static let borderWidth: CGFloat = 1
}

struct OTPCodeField: View {
    @Binding var code: String
    let isFocused: Bool
    var isInvalid = false
    let identifier: String
    let focus: () -> Void
    let onComplete: (String) -> Void

    @ScaledMetric(relativeTo: .title) private var boxHeight = OTPBoxMetrics.height

    var body: some View {
        ZStack {
            HStack(spacing: OTPBoxMetrics.spacing) {
                ForEach(0..<OTPCode.length, id: \.self) { index in
                    box(at: index)
                }
            }
            .accessibilityHidden(true)
            TextField(text: $code, prompt: nil) { EmptyView() }
                .keyboardType(.numberPad)
                .textContentType(.oneTimeCode)
                .foregroundStyle(.clear)
                .tint(.clear)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .frame(maxWidth: .infinity, minHeight: boxHeight)
                .accessibilityLabel("6-digit code")
                .accessibilityIdentifier(identifier)
        }
        .contentShape(Rectangle())
        .onTapGesture { focus() }
        .monacoPrivate()
        .onChange(of: code) { _, newValue in
            let sanitized = String(newValue.filter(\.isASCIIDigit).prefix(OTPCode.length))
            guard sanitized == newValue else {
                code = sanitized
                return
            }
            guard sanitized.count == OTPCode.length else { return }
            onComplete(sanitized)
        }
    }

    private func box(at index: Int) -> some View {
        let digits = Array(code)
        let isCurrent = isFocused && index == min(digits.count, OTPCode.length - 1)
        let shape = RoundedRectangle(cornerRadius: MonacoTheme.Radius.field, style: .continuous)
        return Text(index < digits.count ? String(digits[index]) : "")
            .font(MonacoTheme.Typo.title)
            .foregroundStyle(MonacoTheme.primaryText)
            .dynamicTypeSize(...DynamicTypeSize.accessibility1)
            .frame(maxWidth: .infinity, minHeight: boxHeight)
            .background(shape.fill(MonacoTheme.surface))
            .overlay {
                shape.strokeBorder(
                    borderColor(isCurrent: isCurrent), lineWidth: OTPBoxMetrics.borderWidth)
            }
    }

    private func borderColor(isCurrent: Bool) -> Color {
        if isInvalid { return MonacoTheme.loss }
        return isCurrent ? MonacoTheme.brand : MonacoTheme.border
    }
}

extension Character {
    fileprivate var isASCIIDigit: Bool {
        isASCII && isNumber
    }
}
