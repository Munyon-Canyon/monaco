import MonacoCore
import SwiftUI

struct ReferralCodeSheet: View {
    let attacher: ReferralAttacher
    let userID: String
    let onAttached: () -> Void

    @Environment(ToastCenter.self) private var rootToasts
    @Environment(\.dismiss) private var dismiss
    @State private var model: ReferralEntryModel
    @State private var toasts = ToastCenter()
    @FocusState private var isFocused: Bool

    init(attacher: ReferralAttacher, userID: String, onAttached: @escaping () -> Void) {
        self.attacher = attacher
        self.userID = userID
        self.onAttached = onAttached
        _model = State(initialValue: ReferralEntryModel(attacher: attacher, userID: userID))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            Text("Have a referral code?")
                .font(MonacoTheme.Typo.title)
                .foregroundStyle(MonacoTheme.primaryText)
                .accessibilityAddTraits(.isHeader)
            TextField(
                "Code or invite link", text: $model.text,
                prompt: Text("Code or invite link").foregroundStyle(MonacoTheme.disabledLabel)
            )
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.ink)
            .textInputAutocapitalization(.never)
            .autocorrectionDisabled()
            .keyboardType(.asciiCapable)
            .submitLabel(.done)
            .focused($isFocused)
            .onSubmit { Task { await submit() } }
            .monacoFieldChrome(isFocused: isFocused)
            .accessibilityIdentifier("referral-code-field")

            Button {
                Task { await submit() }
            } label: {
                Text(model.isSubmitting ? "Adding" : "Add code")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(!model.canSubmit)
            .accessibilityIdentifier("referral-code-submit")
        }
        .padding(MonacoTheme.Space.gutter)
        .frame(maxHeight: .infinity, alignment: .top)
        .monacoCanvas()
        .monacoToastCenter(toasts)
        .presentationDetents([.medium])
        .onAppear { isFocused = true }
    }

    private func submit() async {
        switch await model.submit() {
        case .attached(let toast):
            rootToasts.show(success: toast)
            onAttached()
            dismiss()
        case .failed(let toast):
            toasts.current = MonacoToast(message: toast)
        case nil:
            break
        }
    }
}
