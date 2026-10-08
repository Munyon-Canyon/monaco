import MonacoAPI
import MonacoCore
import SwiftUI

enum JoinCabalCopy {
    static let title = "Ask to join"
    static let codeLabel = "Invite code"
}

struct JoinCabalView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: JoinCabalModel?

    init(model: JoinCabalModel? = nil) {
        _model = State(initialValue: model)
    }

    var body: some View {
        ZStack {
            Color.clear
            if let model {
                JoinCabalForm(model: model, join: { await join(model) })
            }
        }
        .monacoCanvas()
        .navigationTitle(JoinCabalCopy.title)
        .navigationBarTitleDisplayMode(.inline)
        .task {
            if model == nil {
                model = JoinCabalModel(api: environment.api)
            }
        }
    }

    private func join(_ model: JoinCabalModel) async {
        guard let joined = await model.submit() else { return }
        Haptics.selection()
        if let toast = joined.toast {
            toasts.show(success: toast)
        }
        let navigator = environment.navigator
        if navigator.cabalsPath.last == AnyAppRoute(JoinRoute()) {
            navigator.cabalsPath.removeLast()
        }
        navigator.open(CabalRoute(id: joined.cabalID), in: .cabals)
        if joined.toast == CabalEntry.requestedToast {
            Task { await environment.pushPrePrompt.noteCabalJoined(after: toasts) }
        }
    }
}

private struct JoinCabalForm: View {
    @Bindable var model: JoinCabalModel
    let join: () async -> Void
    @State private var toast: MonacoToast?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    InviteCodeField(code: $model.code, isDisabled: model.isSubmitting)
                    Text(model.message ?? JoinCabalModel.helper)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(model.isError ? MonacoTheme.warning : MonacoTheme.muted)
                        .fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("join-cabal-message")
                }
                if let preview = model.preview {
                    JoinCabalPreviewRow(preview: preview)
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .scrollDismissesKeyboard(.interactively)
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button {
                    Task { await join() }
                } label: {
                    SubmitLabel(isWorking: model.isSubmitting, idle: model.actionTitle, working: model.actionTitle)
                }
                .buttonStyle(.monacoPrimary)
                .disabled(!model.canSubmit)
                .accessibilityIdentifier("join-group-submit")
            }
        }
        .monacoToast($toast, placement: .aboveBottomCTA)
        .task(id: model.inviteCode) { await model.lookUp() }
        .onChange(of: model.toast) { _, next in
            guard let next else { return }
            toast = MonacoToast(message: next.message, isSuccess: next.isSuccess)
        }
    }
}

private struct JoinCabalPreviewRow: View {
    let preview: Components.Schemas.CabalPreview

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            CabalMark(groupId: preview.id, name: preview.name, size: 56, pictureUrl: preview.pictureUrl)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(preview.name)
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.ink)
                    .lineLimit(2)
                    .minimumScaleFactor(0.8)
                    .accessibilityIdentifier("join-group-name")
                Text(CabalCopy.memberCount(preview.memberCount))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .accessibilityElement(children: .combine)
    }
}

/// The invite code field: the `MonacoTextField` anatomy, with the code in the market's mono —
/// a code is data, not words — and the system Paste button inside it, which reads the
/// clipboard without the paste prompt.
private struct InviteCodeField: View {
    @Binding var code: String
    let isDisabled: Bool

    @FocusState private var focused: Bool

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            TextField(
                "",
                text: $code,
                prompt: Text(JoinCabalCopy.codeLabel).foregroundStyle(MonacoTheme.disabledLabel)
            )
            .font(MonacoTheme.Typo.data)
            .foregroundStyle(MonacoTheme.ink)
            .tint(MonacoTheme.ink)
            .keyboardType(.asciiCapable)
            .textInputAutocapitalization(.never)
            .autocorrectionDisabled()
            .submitLabel(.done)
            .focused($focused)
            .disabled(isDisabled)
            .frame(maxWidth: .infinity, minHeight: 56)
            .contentShape(Rectangle())
            .onTapGesture { focused = true }
            .accessibilityLabel(JoinCabalCopy.codeLabel)
            .accessibilityIdentifier("join-group-id")

            PasteButton(payloadType: String.self) { strings in
                guard let pasted = strings.first else { return }
                Task { @MainActor in
                    code = pasted.trimmingCharacters(in: .whitespacesAndNewlines)
                }
            }
            .labelStyle(.iconOnly)
            .buttonBorderShape(.capsule)
            // The Paste button draws its glyph in white on the tint in both schemes, so the tint
            // is the ink that stays dark in both rather than `brandFill`, which goes light in dark.
            .tint(MonacoTheme.heroInk)
            .disabled(isDisabled)
            .accessibilityIdentifier("join-group-paste")
        }
        .padding(.leading, MonacoTheme.Space.m)
        .padding(.trailing, MonacoTheme.Space.s)
        .background(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.field, style: .continuous)
                .fill(MonacoTheme.surfaceSunken)
        )
        .overlay {
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.field, style: .continuous)
                .strokeBorder(MonacoTheme.ink, lineWidth: focused ? 1 : 0)
        }
        .animation(.easeOut(duration: 0.15), value: focused)
    }
}

#if DEBUG
final class JoinCabalSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-MonacoJoinCabalSample"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains(launchArgument) else { return nil }
        return AnyView(
            NavigationStack {
                JoinCabalView(model: .preview())
            }
        )
    }
}
#endif
