#if DEBUG
import SwiftUI

struct GalleryFieldSamples: View {
    @State private var empty = ""
    @State private var query = "Apple"
    @State private var short = "ab"
    @State private var email = ""
    @State private var thesis = "Mostly semis, nothing before earnings."
    @State private var ready = "123"
    @State private var rejected = "123456"

    var body: some View {
        GalleryGroup(title: "Fields") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                search
                text
                GalleryCase(caption: "OTP, ready") {
                    OTPCodeField(
                        code: $ready, isFocused: false, identifier: "gallery-otp-ready", focus: {},
                        onComplete: { _ in })
                }
                GalleryCase(caption: "OTP, invalid") {
                    OTPCodeField(
                        code: $rejected, isFocused: false, isInvalid: true, identifier: "gallery-otp-invalid",
                        focus: {}, onComplete: { _ in })
                }
            }
        }
    }

    private var search: some View {
        GalleryCase(caption: "Search, empty and filled") {
            VStack(spacing: MonacoTheme.Space.s) {
                MonacoSearchField(placeholder: "Search Apple, Tesla, NVDA…", text: $empty)
                MonacoSearchField(placeholder: "Search Apple, Tesla, NVDA…", text: $query)
            }
        }
    }

    private var text: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            GalleryCase(caption: "Field, default") {
                MonacoTextField("Cabal name", text: $empty, label: "Name")
            }
            GalleryCase(caption: "Field, focused") {
                Text("Weekend investors")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.ink)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .monacoFieldChrome(isFocused: true, focus: {})
            }
            GalleryCase(caption: "Field, invalid") {
                MonacoTextField(
                    "Cabal name", text: $short, isInvalid: true, errorMessage: "Use at least 3 characters",
                    label: "Name")
            }
            GalleryCase(caption: "Field, helper") {
                MonacoTextField(
                    "Email", text: $email, keyboard: .emailAddress, label: "Email",
                    helper: "We only use it to sign you in")
            }
            GalleryCase(caption: "Field, multiline") {
                MonacoTextField("Why this stock?", text: $thesis, label: "Thesis", isMultiline: true)
            }
        }
    }
}

struct GalleryAmountSamples: View {
    @State private var ready = "50"
    @State private var over = "600"
    @State private var typed = "12.5"

    private let presets: [AmountPreset] = [.dollars(25), .dollars(50), .dollars(100), .fraction(1, label: "Max")]

    var body: some View {
        GalleryGroup(title: "Amount") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Amount entry, ready") {
                    AmountEntry(
                        amountText: $ready, max: Decimal(string: "548.20"), presets: presets,
                        helper: "The pot has $548.20", overLimitHelper: "More than the pot has", input: .keypad)
                }
                GalleryCase(caption: "Amount entry, over limit") {
                    AmountEntry(
                        amountText: $over, max: Decimal(string: "548.20"), presets: presets,
                        helper: "The pot has $548.20", overLimitHelper: "More than the pot has", input: .keypad)
                }
                GalleryCase(caption: "Keypad") {
                    AmountKeypad(amountText: $typed)
                }
            }
        }
    }
}
#endif
