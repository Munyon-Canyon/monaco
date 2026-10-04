import SwiftUI

struct OTPCodeField: View {
    @Binding var code: String
    let isFocused: Bool
    var isInvalid = false
    let identifier: String
    let onComplete: (String) -> Void

    var body: some View {
        TextField(
            "6-digit code",
            text: $code,
            prompt: Text("6-digit code")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.disabledLabel)
        )
        .font(MonacoTheme.Typo.data)
        .tracking(code.isEmpty ? 0 : OTPCode.tracking)
        .keyboardType(.numberPad)
        .textContentType(.oneTimeCode)
        .authTextFieldStyle(isFocused: isFocused, isInvalid: isInvalid)
        .accessibilityIdentifier(identifier)
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
}

extension Character {
    fileprivate var isASCIIDigit: Bool {
        isASCII && isNumber
    }
}
