#if DEBUG
import MonacoCore
import SwiftUI

/// Dev builds only: text-message sign-in through the Privy dev app, so agents and simulators can
/// sign in without an Apple or Google account. Production never offers it.
struct SMSLoginView: View {
    @ObservedObject var session: OTPSession
    let scroll: ScrollViewProxy
    /// Debug harness only: the code the form opens with.
    var initialCode = ""

    var body: some View {
        OTPLoginForm(
            session: session,
            destination: .sms,
            scroll: scroll,
            initialCode: initialCode
        )
    }
}

/// The dev text-message login against Privy.
final class PrivySMSLogin: OTPSession {
    private let auth: PrivyAuthService

    init(auth: PrivyAuthService) {
        self.auth = auth
    }

    override func deliverCode(to destination: String) async throws {
        try await auth.sendSMSCode(to: destination)
    }

    override func redeemCode(_ code: String, sentTo destination: String) async throws {
        try await auth.loginWithSMSCode(code, sentTo: destination)
        // Privy took the code but the session token could not be fetched; the login screen
        // toasts why, and the code box stays for another try.
        guard case .authenticated = auth.phase else { throw LoginFailure.other(detail: nil) }
    }
}

extension OTPDestination {
    static let sms = OTPDestination(
        caption: "We'll text you a code to sign in.",
        prompt: "Phone number",
        keyboardType: .phonePad,
        contentType: .telephoneNumber,
        invalidHint: "Enter a mobile number with its country code, like +1 555 123 4567.",
        changeLabel: "Change number",
        addressFieldIdentifier: "smsPhoneField",
        identifierPrefix: "sms",
        normalize: { E164PhoneNumber($0)?.value },
        display: PhoneReadBack.format
    )
}

/// How a number reads back once a code has gone to it.
///
/// A North American number reads "+1 555 123 4567": the shape the field's own hint asks for, with
/// the country code that says it went to the right country. Any other number reads in its E.164
/// form, because where its spaces go depends on a country this app does not look up.
enum PhoneReadBack {
    static func format(_ e164: String) -> String {
        let digits = e164.dropFirst()
        guard e164.hasPrefix("+1"),
              digits.count == 11,
              digits.allSatisfy({ $0.isASCII && $0.isNumber })
        else { return e164 }
        let national = Array(digits.dropFirst())
        return "+1 \(String(national[0..<3])) \(String(national[3..<6])) \(String(national[6..<10]))"
    }
}

#Preview {
    ScrollViewReader { proxy in
        SMSLoginView(session: PrivySMSLogin(auth: PrivyAuthService()), scroll: proxy)
            .padding()
    }
}
#endif
