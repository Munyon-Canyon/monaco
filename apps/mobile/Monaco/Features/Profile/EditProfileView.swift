import MonacoAPI
import MonacoCore
import SwiftUI

enum EditProfileFocus {
    case username
}

enum EditProfilePhase {
    case saving
    case saved
}

struct EditProfilePresets {
    var nameDraft: String?
    var handleDraft: String?
    var bioDraft: String?
    var bioEnabled = false
    var phase: EditProfilePhase?
    var loadFailed = false
    var showsFacePicker = false
    var saveName: (any DisplayNameSaving)?
    var sessions: SessionAPI?
}

extension EnvironmentValues {
    @Entry var editProfilePresets = EditProfilePresets()
}

enum EditProfileCopy {
    static let title = "Edit profile"
    static let invalidUsername = "Choose a valid username."
    static let saved = "Profile saved."
    static let saving = "Saving…"
    static let save = "Save changes"
    static let bioHelper = "Bio opens soon."
    static let failedTitle = "Couldn't load this view"
    static let failedMessage =
        "Your existing information hasn't been changed. Try again when your connection is available."
}

enum EditProfileMetrics {
    static let photoBlockHeight: CGFloat = 90
}

struct EditProfileView: View {
    var focus: EditProfileFocus?

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.editProfilePresets) private var presets

    init(focus: EditProfileFocus? = nil) {
        self.focus = focus
    }

    var body: some View {
        EditProfileForm(
            focus: focus, presets: presets, sessions: presets.sessions ?? SessionAPI(api: environment.api))
    }
}

private struct EditProfileForm: View {
    let focus: EditProfileFocus?
    let presets: EditProfilePresets
    let sessions: SessionAPI

    @EnvironmentObject private var auth: PrivyAuthService
    @Environment(AppSessionStore.self) private var session
    @Environment(ToastCenter.self) private var toasts

    @State private var checker: HandleAvailabilityChecker
    @State private var status = HandleStatus.idle
    @State private var nameDraft = ""
    @State private var handleDraft = ""
    @State private var bioDraft = ""
    @State private var nameSaveError: String?
    @State private var isSaving = false
    @State private var savedSnapshot: String?
    @State private var confirmingHandle: String?
    @State private var nameSubmission = IdempotentSubmission()
    @State private var handleSubmissions: [String: IdempotentSubmission] = [:]

    init(focus: EditProfileFocus?, presets: EditProfilePresets, sessions: SessionAPI) {
        self.focus = focus
        self.presets = presets
        self.sessions = sessions
        _checker = State(initialValue: HandleAvailabilityChecker(sessions: sessions, clock: ContinuousClock()))
    }

    private var profile: SessionProfile? { session.profile }

    private var savedName: String { profile?.displayName ?? "" }

    private var isLocked: Bool {
        profile?.handleChangeableAt.map { $0 > .now } ?? false
    }

    private var normalizedName: String? { try? DisplayNameRules.normalize(nameDraft).get() }

    private var nameChanged: Bool { normalizedName.map { $0 != savedName } ?? false }

    private var nameError: String? {
        DisplayNameRules.validationMessage(for: nameDraft) ?? nameSaveError
    }

    private var normalizedHandle: String { HandleInput.normalize(handleDraft) }

    private var handleChanged: Bool {
        !isLocked && normalizedHandle != (profile?.handle ?? "")
    }

    private var handleError: String? {
        guard handleChanged else { return nil }
        let handle = normalizedHandle
        if handle.isEmpty || HandleInput.isShortButValid(handle) { return EditProfileCopy.invalidUsername }
        switch status {
        case .unavailable(let checked, let reason) where checked == handle:
            return reason == .invalid
                ? EditProfileCopy.invalidUsername : reason.message(changeableAt: profile?.handleChangeableAt)
        case .failed(let checked) where checked == handle:
            return HandleCopy.checkFailed
        default:
            return nil
        }
    }

    private var handleHelper: String? {
        if isLocked {
            return HandleReason.tooSoon.message(changeableAt: profile?.handleChangeableAt)
        }
        guard handleChanged, handleError == nil else { return nil }
        switch status {
        case .checking(let checked) where checked == normalizedHandle: return HandleCopy.checking
        case .available(let checked) where checked == normalizedHandle: return HandleCopy.available
        case .slowDown(let checked) where checked == normalizedHandle: return HandleCopy.slowDown
        default: return nil
        }
    }

    private var handleReady: Bool {
        !handleChanged || status.claimable == normalizedHandle
    }

    private var showsSaving: Bool { isSaving || presets.phase == .saving }

    private var showsSaved: Bool {
        presets.phase == .saved || (savedSnapshot != nil && savedSnapshot == fingerprint)
    }

    private var fingerprint: String {
        "\(normalizedName ?? nameDraft)|\(normalizedHandle)|\(bioDraft)"
    }

    private var canSave: Bool {
        guard profile != nil, !showsSaving else { return false }
        guard DisplayNameRules.validationMessage(for: nameDraft) == nil, handleError == nil, handleReady else {
            return false
        }
        return nameChanged || handleChanged
    }

    var body: some View {
        Group {
            if presets.loadFailed || (profile == nil && !session.isLoading) {
                failure
            } else if profile == nil {
                skeleton
            } else {
                form
            }
        }
        .monacoCanvas()
        .navigationTitle(EditProfileCopy.title)
        .navigationBarTitleDisplayMode(.inline)
        .task { for await next in await checker.statuses() { status = next } }
        .task(id: handleDraft) {
            guard !isLocked else { return }
            await checker.update(handleDraft)
        }
        .onChange(of: profile?.userID, initial: true) { _, _ in seed() }
        .onChange(of: nameDraft) { _, _ in nameSaveError = nil }
        .confirmationDialog(
            HandleCopy.changeTitle(to: confirmingHandle ?? ""),
            isPresented: Binding(get: { confirmingHandle != nil }, set: { if !$0 { confirmingHandle = nil } }),
            titleVisibility: .visible
        ) {
            Button(HandleCopy.changeConfirm) { Task { await save() } }
            Button(HandleCopy.cancel, role: .cancel) {}
        } message: {
            Text(HandleCopy.changeMessage(from: profile?.handle ?? ""))
        }
    }

    private var form: some View {
        ScrollView {
            VStack(spacing: 0) {
                photo
                    .frame(maxWidth: .infinity, minHeight: EditProfileMetrics.photoBlockHeight)
                    .padding(.top, MonacoTheme.Space.sm)
                    .padding(.bottom, MonacoTheme.Space.l)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    nameField
                    usernameField
                    bioField
                    notice
                }
                saveButton
                    .padding(.top, MonacoTheme.Space.l)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.l)
            .disabled(showsSaving)
        }
        .scrollDismissesKeyboard(.interactively)
    }

    private func save() async {
        guard canSave else { return }
        isSaving = true
        nameSaveError = nil
        defer { isSaving = false }
        if nameChanged {
            let outcome: ProfileSaveOutcome
            if let saveName = presets.saveName {
                outcome = await saveName.saveDisplayName(nameDraft)
            } else {
                outcome = await session.updateDisplayName(nameDraft, auth: auth, submission: nameSubmission)
            }
            switch outcome {
            case .saved:
                nameDraft = savedName
            case .unchanged:
                break
            case .failed(let message):
                nameSaveError = message
                toasts.current = MonacoToast(message: message)
                return
            }
        }
        if handleChanged {
            let handle = normalizedHandle
            let submission = handleSubmissions[handle] ?? IdempotentSubmission()
            handleSubmissions[handle] = submission
            do {
                let saved = try await sessions.setHandle(handle, submission: submission)
                session.profile = saved
                handleDraft = saved.handle ?? handle
            } catch {
                switch HandleSaveFailure(APIError(error)) {
                case .inline(let reason):
                    status = .unavailable(handle, reason)
                case .toast(let message):
                    toasts.current = MonacoToast(message: message)
                }
                return
            }
        }
        savedSnapshot = fingerprint
        toasts.show(success: EditProfileCopy.saved)
        AccessibilityNotification.Announcement(EditProfileCopy.saved).post()
    }
}

extension EditProfileForm {
    private var photo: some View {
        ProfilePhotoPicker(
            auth: auth,
            size: MonacoAvatarSize.edit.rawValue,
            badge: .pencil(ring: MonacoTheme.canvas),
            initiallyOpen: presets.showsFacePicker
        ) { toasts.current = $0 }
    }

    private var nameField: some View {
        MonacoTextField(
            "Name",
            text: $nameDraft,
            isInvalid: nameError != nil,
            errorMessage: nameError,
            capitalization: .words,
            autocorrects: false,
            label: "Name"
        )
        .submitLabel(.done)
        .accessibilityIdentifier("profile-name-field")
    }

    private var usernameField: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoTextField(
                "Username",
                text: $handleDraft,
                keyboard: .asciiCapable,
                isInvalid: handleError != nil,
                errorMessage: handleError,
                autofocus: focus == .username && !isLocked,
                capitalization: .never,
                autocorrects: false,
                label: "Username",
                helper: handleHelper
            )
            .submitLabel(.done)
            .disabled(isLocked)
            .accessibilityIdentifier("edit-profile-username-field")
            if case .failed = status, handleChanged {
                Button(HandleCopy.tryAgain) { Task { await checker.retry() } }
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier("edit-profile-username-retry")
            }
        }
    }

    private var bioField: some View {
        MonacoTextField(
            "",
            text: $bioDraft,
            label: "Bio",
            helper: presets.bioEnabled ? nil : EditProfileCopy.bioHelper,
            isMultiline: true
        )
        .disabled(!presets.bioEnabled)
        .accessibilityIdentifier("edit-profile-bio-field")
    }

    @ViewBuilder private var notice: some View {
        if showsSaving {
            MonacoNotice(.info, message: EditProfileCopy.saving)
                .accessibilityIdentifier("edit-profile-saving")
        } else if showsSaved {
            MonacoNotice(.info, message: EditProfileCopy.saved)
                .accessibilityIdentifier("edit-profile-saved")
        }
    }

    private var saveButton: some View {
        Button(EditProfileCopy.save) {
            if handleChanged {
                confirmingHandle = normalizedHandle
            } else {
                Task { await save() }
            }
        }
        .buttonStyle(.monacoPrimary)
        .disabled(!canSave)
        .accessibilityIdentifier("profile-name-save")
    }

    private var skeleton: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            SkeletonBlock(
                width: MonacoAvatarSize.edit.rawValue, height: MonacoAvatarSize.edit.rawValue,
                radius: MonacoAvatarSize.edit.rawValue / 2)
            SkeletonBlock(height: MonacoFieldChrome.height)
            SkeletonBlock(height: MonacoFieldChrome.height)
            SkeletonBlock(height: MonacoFieldChrome.multilineHeight)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .frame(maxHeight: .infinity, alignment: .top)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading your profile")
        .accessibilityIdentifier("edit-profile-loading")
    }

    private var failure: some View {
        EmptyState(
            title: EditProfileCopy.failedTitle,
            message: EditProfileCopy.failedMessage,
            actionTitle: "Try again",
            actionIdentifier: "edit-profile-error",
            systemImage: MonacoIcon.offline.symbol,
            action: { Task { await session.reloadProfile(auth: auth) } }
        )
        .frame(maxHeight: .infinity)
    }

    private func seed() {
        guard let profile else { return }
        nameDraft = presets.nameDraft ?? profile.displayName
        handleDraft = presets.handleDraft ?? profile.handle ?? ""
        bioDraft = presets.bioDraft ?? ""
        nameSaveError = nil
        savedSnapshot = nil
    }
}
