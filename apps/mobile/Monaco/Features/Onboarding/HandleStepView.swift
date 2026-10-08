import MonacoAPI
import MonacoCore
import SwiftUI

struct HandleStepView: View {
    enum Mode {
        case onboarding
        case edit
    }

    @Environment(AppEnvironment.self) private var environment

    var mode = Mode.onboarding
    var onContinue: () -> Void = {}
    var sessions: SessionAPI?
    var draft = ""
    var continuesWhenAvailable = false

    var body: some View {
        HandleStepForm(
            mode: mode, sessions: sessions ?? SessionAPI(api: environment.api), initialDraft: draft,
            continuesWhenAvailable: continuesWhenAvailable, referrals: environment.referrals, onContinue: onContinue,
            onSignOut: { await environment.signOut() })
    }
}

private struct HandleStepForm: View {
    @Environment(AppSessionStore.self) private var session
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss

    let mode: HandleStepView.Mode
    let sessions: SessionAPI
    let continuesWhenAvailable: Bool
    let referrals: ReferralAttacher
    let onContinue: () -> Void
    let onSignOut: () async -> Void

    @State private var checker: HandleAvailabilityChecker
    @State private var typed = AsyncStream.makeStream(of: String.self)
    @State private var draft: String
    @State private var status = HandleStatus.idle
    @State private var isSaving = false
    @State private var submissions: [String: IdempotentSubmission] = [:]
    @State private var offersReferralEntry = false
    @State private var isEnteringReferral = false
    @FocusState private var isFocused: Bool

    init(
        mode: HandleStepView.Mode, sessions: SessionAPI, initialDraft: String, continuesWhenAvailable: Bool,
        referrals: ReferralAttacher, onContinue: @escaping () -> Void, onSignOut: @escaping () async -> Void
    ) {
        self.mode = mode
        self.sessions = sessions
        self.continuesWhenAvailable = continuesWhenAvailable
        self.referrals = referrals
        self.onContinue = onContinue
        self.onSignOut = onSignOut
        _checker = State(initialValue: HandleAvailabilityChecker(sessions: sessions, clock: ContinuousClock()))
        _draft = State(initialValue: initialDraft)
    }

    private var profile: SessionProfile? { session.profile }

    private var isLocked: Bool {
        mode == .edit && (profile?.handleChangeableAt.map { $0 > .now } ?? false)
    }

    private var claimable: String? {
        guard !isSaving, !isLocked, let handle = status.claimable, handle == HandleInput.normalize(draft) else {
            return nil
        }
        return mode == .edit && handle == profile?.handle ? nil : handle
    }

    private var subtext: String {
        guard mode == .onboarding, let profile else { return HandleStepReason.firstRun.subtext }
        return HandleInput.stepReason(for: profile).subtext
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    Text(OnboardingCopy.handleTitle)
                        .font(MonacoTheme.Typo.display)
                        .foregroundStyle(MonacoTheme.ink)
                        .accessibilityAddTraits(.isHeader)
                    Text(subtext)
                        .font(MonacoTheme.Typo.callout)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .accessibilityIdentifier("handle-step-subtext")
                }
                .fixedSize(horizontal: false, vertical: true)

                field
                statusLine
                if offersReferralEntry {
                    Button {
                        isEnteringReferral = true
                    } label: {
                        Text("Have a referral code?")
                    }
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier("onboarding-handle-step-referral")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xl)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .scrollDismissesKeyboard(.interactively)
        .safeAreaInset(edge: .bottom) { BottomCTA { continueButton } }
        .monacoCanvas()
        .navigationTitle(mode == .edit ? "Handle" : "")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            FirstRunToolbar(
                signOut: mode == .onboarding
                    ? FirstRunAction(
                        title: OnboardingCopy.signOut, identifier: "onboarding-handle-step-sign-out",
                        isDisabled: isSaving
                    ) { Task { await onSignOut() } } : nil,
                skip: nil)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("onboarding-handle-step")
        .task { for await next in checker.statuses { status = next } }
        .task {
            for await text in typed.stream { await checker.update(text) }
        }
        .onChange(of: draft, initial: true) { _, text in
            guard !isLocked else { return }
            typed.continuation.yield(text)
        }
        .onChange(of: status) { _, _ in
            if continuesWhenAvailable, claimable != nil { Task { await save() } }
        }
        .onAppear {
            if mode == .edit, draft.isEmpty, let handle = profile?.handle { draft = handle }
            if !isLocked { isFocused = true }
            refreshReferralEntry()
        }
        .sheet(isPresented: $isEnteringReferral) {
            if let userID = profile?.userID {
                ReferralCodeSheet(attacher: referrals, userID: userID, onAttached: refreshReferralEntry)
            }
        }
    }

    private func refreshReferralEntry() {
        guard mode == .onboarding, let userID = profile?.userID else {
            offersReferralEntry = false
            return
        }
        offersReferralEntry = referrals.offersManualEntry(userID: userID)
    }

    private var field: some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Text("@")
                .font(MonacoTheme.Typo.bodyStrong)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityHidden(true)
            TextField("handle", text: $draft, prompt: Text("handle").foregroundStyle(MonacoTheme.disabledLabel))
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .tint(MonacoTheme.ink)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .keyboardType(.asciiCapable)
                .submitLabel(.continue)
                .focused($isFocused)
                .onSubmit { Task { await save() } }
                .accessibilityLabel("Handle")
                .accessibilityIdentifier("handle-step-field")
        }
        .monacoFieldChrome(isFocused: isFocused, isInvalid: isInvalid)
        .disabled(isLocked || isSaving)
        .opacity(isLocked ? 0.6 : 1)
    }

    private var isInvalid: Bool {
        if case .unavailable = status { return true }
        return isLocked
    }

    @ViewBuilder
    private var statusLine: some View {
        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.xs) {
            switch shownStatus {
            case .idle:
                Text(" ")
            case .checking:
                ProgressView().controlSize(.mini)
                Text(HandleCopy.checking).foregroundStyle(MonacoTheme.secondaryText)
            case .available:
                Image(systemName: "checkmark.circle.fill").foregroundStyle(MonacoTheme.success)
                Text(HandleCopy.available).foregroundStyle(MonacoTheme.success)
            case .unavailable(_, let reason):
                Text(reason.message(changeableAt: profile?.handleChangeableAt)).foregroundStyle(MonacoTheme.loss)
            case .slowDown:
                Text(HandleCopy.slowDown).foregroundStyle(MonacoTheme.secondaryText)
            case .failed:
                Text(HandleCopy.checkFailed).foregroundStyle(MonacoTheme.loss)
                Button(HandleCopy.tryAgain) { Task { await checker.retry() } }
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier("handle-step-try-again")
            }
        }
        .font(MonacoTheme.Typo.caption)
        .fixedSize(horizontal: false, vertical: true)
        .frame(minHeight: 44, alignment: .leading)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(statusLabel)
        .accessibilityIdentifier("handle-step-status")
    }

    private var shownStatus: HandleStatus {
        isLocked ? .unavailable(profile?.handle ?? "", .tooSoon) : status
    }

    private var statusLabel: String {
        switch shownStatus {
        case .idle: ""
        case .checking: HandleCopy.checking
        case .available(let handle): "@\(handle) is available"
        case .unavailable(_, let reason): reason.message(changeableAt: profile?.handleChangeableAt)
        case .slowDown: HandleCopy.slowDown
        case .failed: HandleCopy.checkFailed
        }
    }

    private var continueButton: some View {
        Button {
            Task { await save() }
        } label: {
            SubmitLabel(isWorking: isSaving, idle: OnboardingCopy.continueLabel, working: HandleCopy.saving)
        }
        .buttonStyle(.monacoPrimary)
        .disabled(claimable == nil)
        .accessibilityValue(claimable == nil && !isSaving ? "Unavailable" : "")
        .accessibilityIdentifier("onboarding-handle-step-continue")
    }

    private func save() async {
        guard let handle = claimable else { return }
        let submission = submissions[handle] ?? IdempotentSubmission()
        submissions[handle] = submission
        isSaving = true
        defer { isSaving = false }
        do {
            let saved = try await sessions.setHandle(handle, submission: submission)
            session.profile = saved
            switch mode {
            case .onboarding:
                onContinue()
            case .edit:
                toasts.show(success: HandleCopy.updated)
                dismiss()
            }
        } catch {
            switch HandleSaveFailure(APIError(error)) {
            case .inline(let reason):
                status = .unavailable(handle, reason)
            case .toast(let message):
                toasts.current = MonacoToast(message: message)
            }
        }
    }
}

#if DEBUG
enum HandleSampleScenario: String, CaseIterable {
    case firstRun
    case revoked
    case checking
    case checkFailed
    case saving
    case editLocked

    static func matching(_ arguments: [String]) -> HandleSampleScenario? {
        guard let flag = arguments.firstIndex(of: "-MonacoHandleSample"), arguments.indices.contains(flag + 1)
        else { return nil }
        return HandleSampleScenario(rawValue: arguments[flag + 1])
    }

    var answer: HandlePreviewAnswer {
        switch self {
        case .firstRun, .revoked, .editLocked: .available
        case .checking: .checking
        case .checkFailed: .checkFailed
        case .saving: .saving
        }
    }

    var draft: String {
        switch self {
        case .firstRun, .revoked, .editLocked: ""
        case .checking, .checkFailed, .saving: "qa_handle_1"
        }
    }
}

final class HandleSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let scenario = HandleSampleScenario.matching(arguments) else { return nil }
        return AnyView(HandleSampleHarness(scenario: scenario))
    }
}

private struct HandleSampleHarness: View {
    let scenario: HandleSampleScenario
    @State private var session = AppSessionStore()

    var body: some View {
        NavigationStack {
            HandleStepView(
                mode: scenario == .editLocked ? .edit : .onboarding,
                sessions: .handlePreview(scenario.answer),
                draft: scenario.draft,
                continuesWhenAvailable: scenario == .saving
            )
        }
        .environment(session)
        .onAppear {
            var profile = SessionProfile(Components.Schemas.Me.sample)
            profile.handle = scenario == .editLocked ? profile.handle : nil
            switch scenario {
            case .revoked: profile.authState = .awaitingPhone
            case .editLocked: break
            default: profile.authState = .created
            }
            profile.handleChangeableAt = .now.addingTimeInterval(20 * 24 * 3600)
            session.isLoading = false
            session.profile = profile
        }
    }
}

#endif
