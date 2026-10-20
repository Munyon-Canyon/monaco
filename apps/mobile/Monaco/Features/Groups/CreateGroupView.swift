import MonacoAPI
import MonacoCore
import PhotosUI
import SwiftUI

enum CabalRulesCopy {
    static let screenTitle = "Start a cabal"
    static let namePlaceholder = "Cabal name"
    static let nameHint = "Pick a name your friends will recognize."
    static let createPickedCaption = "Only you vote until you add people in Cabal settings."
    static let createShowsThreshold = false
    static let sectionTitle = "The rules"
    static let joinTitle = "Who can join"
    static let votersTitle = "Who votes"
    static let thresholdTitle = "To pass"
    static let expiryTitle = "Votes stay open"
    static let create = "Create cabal"
    static let creating = "Creating…"
    static let created = "Cabal created."
    static let nameLabel = "Cabal name"
    static let createNamePlaceholder = "A name your friends will recognize"
    static let coverTitle = "Cover image"
    static let coverAdd = "Add a cover photo"
    static let coverHelper = "Choose a cover your cabal will recognize."
    static let joinHelper = "Let people join, or review their requests."
    static let votersHelper = "Choose who participates in cabal decisions."
    static let expiryHelper = "A proposal that hasn't passed closes at this deadline."
    static let votersPickTitle = "Choose voters"
    static let votersPickSubtitle = "Only you vote until you add people in Cabal details."
    static let creatingTitle = "Creating your cabal"
    static let creatingMessage = "Saving the cabal name and decision rules."
    static let createdWithoutCover = "Cabal created. The cover didn't upload. Add it from Cabal details."
    static let createFailed = "Couldn't create your cabal. Your choices are still here."

    static var auditedStrings: [String] {
        [
            screenTitle, namePlaceholder, nameHint, sectionTitle, joinTitle, votersTitle, thresholdTitle, expiryTitle,
            create, creating, created, createPickedCaption, nameLabel, createNamePlaceholder, coverTitle, coverAdd,
            coverHelper, joinHelper, votersHelper, expiryHelper, votersPickTitle, votersPickSubtitle, creatingTitle,
            creatingMessage, createdWithoutCover, createFailed,
        ]
            + CabalJoinPolicy.allCases.flatMap { [$0.label, $0.caption] }
            + CabalVoterMode.allCases.flatMap { [$0.label, $0.caption] }
            + CabalThreshold.allCases.flatMap { [$0.label, $0.caption] }
            + CabalProposalExpiry.allCases.flatMap { [$0.label, $0.caption] }
            + [CreateCabalForm.NameProblem.tooShort, .tooLong, .invalid].compactMap(\.message)
    }
}

extension CreateCabalForm.NameProblem {
    var isHard: Bool { self == .tooLong || self == .invalid }
}

struct CreateGroupView: View {
    @Environment(AppEnvironment.self) private var environment: AppEnvironment?
    @Environment(ToastCenter.self) private var toasts: ToastCenter?

    private let injectedActions: CabalsActionSource?
    private let onCreated: ((Components.Schemas.Cabal) -> Void)?
    private let autofocus: Bool

    @State private var form: CreateCabalForm
    @State private var submission = IdempotentSubmission()
    @State private var isCreating: Bool
    @State private var submitFailed: Bool
    @State private var cover: PhotosPickerItem?
    @State private var coverImage: UIImage?
    @State private var coverUpload: ProfilePhotoUploadPreparer.Prepared?

    init(
        actions: CabalsActionSource? = nil,
        onCreated: ((Components.Schemas.Cabal) -> Void)? = nil,
        form: CreateCabalForm = CreateCabalForm(),
        isCreating: Bool = false,
        submitFailed: Bool = false,
        autofocus: Bool = true
    ) {
        self.autofocus = autofocus
        self.injectedActions = actions
        self.onCreated = onCreated
        _form = State(initialValue: form)
        _isCreating = State(initialValue: isCreating)
        _submitFailed = State(initialValue: submitFailed)
    }

    private var actions: CabalsActionSource? {
        if let injectedActions { return injectedActions }
        guard let environment else { return nil }
        return LiveCabalsActionSource(auth: environment.auth, api: environment.api)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                if isCreating {
                    OutcomeView(
                        tone: .pending, title: CabalRulesCopy.creatingTitle, message: CabalRulesCopy.creatingMessage
                    )
                    .padding(.bottom, MonacoTheme.Space.l)
                    .accessibilityIdentifier("create-group-creating")
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                    nameField
                    coverPicker
                    ruleBlock(
                        title: CabalRulesCopy.joinTitle, helper: CabalRulesCopy.joinHelper,
                        options: CabalJoinPolicy.allCases, selection: $form.joinPolicy, identifier: "create-rule-join"
                    ) { $0.label }
                    ruleBlock(
                        title: CabalRulesCopy.votersTitle, helper: CabalRulesCopy.votersHelper,
                        options: CabalVoterMode.allCases, selection: $form.voterMode, identifier: "create-rule-voters"
                    ) { $0.label }
                    if form.voterMode == .picked {
                        chooseVoters
                            .padding(.top, -MonacoTheme.Space.sm)
                    }
                    ruleBlock(
                        title: CabalRulesCopy.expiryTitle, helper: CabalRulesCopy.expiryHelper,
                        options: CabalProposalExpiry.allCases, selection: $form.expiry,
                        identifier: "create-rule-expiry"
                    ) { $0.label }
                    MonacoNotice(.info, message: form.threshold.caption)
                        .accessibilityIdentifier("create-group-threshold-notice")
                    if submitFailed {
                        MonacoNotice(.error, message: CabalRulesCopy.createFailed)
                            .accessibilityIdentifier("create-group-failed")
                    }
                }
                .disabled(isCreating)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .scrollDismissesKeyboard(.interactively)
        .monacoCanvas()
        .navigationTitle(CabalRulesCopy.screenTitle)
        .navigationBarTitleDisplayMode(.inline)
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button {
                    Task { await create() }
                } label: {
                    SubmitLabel(isWorking: isCreating, idle: CabalRulesCopy.create, working: CabalRulesCopy.creating)
                }
                .buttonStyle(.monacoPrimary)
                .disabled(isCreating || form.input == nil || actions == nil)
                .accessibilityIdentifier("create-group-submit")
            }
        }
        .onChange(of: form) { _, _ in
            submission = IdempotentSubmission()
            submitFailed = false
        }
        .onChange(of: cover) { _, item in
            guard let item else { return }
            Task { await prepareCover(item) }
        }
    }

    private var nameField: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoTextField(
                CabalRulesCopy.createNamePlaceholder,
                text: $form.name,
                isInvalid: isHardProblem,
                isOutlined: true,
                autofocus: autofocus,
                capitalization: .words,
                autocorrects: false,
                label: CabalRulesCopy.nameLabel
            )
            .submitLabel(.done)
            .accessibilityIdentifier("create-group-name")
            if let problem = nameProblemMessage {
                Text(problem)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(isHardProblem ? MonacoTheme.loss : MonacoTheme.secondaryText)
                    .accessibilityIdentifier("create-group-name-problem")
            } else {
                Text(CabalRulesCopy.nameHint)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
    }

    private var coverPicker: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text(CabalRulesCopy.coverTitle)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.primaryText)
            let preview = coverImage
            PhotosPicker(selection: $cover, matching: .images, photoLibrary: .shared()) {
                CreateCoverPreview(image: preview)
            }
            .buttonStyle(.plain)
            .contextMenu {
                if coverImage != nil {
                    Button(role: .destructive) {
                        removeCover()
                    } label: {
                        Label("Remove photo", systemImage: MonacoIcon.trash.symbol)
                    }
                }
            }
            .accessibilityLabel("Cover image, \(coverImage == nil ? "none" : "chosen")")
            .accessibilityIdentifier("create-cover-picker")
            Text(CabalRulesCopy.coverHelper)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
    }

    private func ruleBlock<Option: Hashable>(
        title: String, helper: String, options: [Option], selection: Binding<Option>, identifier: String,
        label: @escaping (Option) -> String
    ) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(title)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .accessibilityAddTraits(.isHeader)
            Text(helper)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, MonacoTheme.Space.xs)
            CreateRuleChoice(options: options, selection: selection, label: label)
                .padding(.top, MonacoTheme.Space.sm)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(identifier)
    }

    private var chooseVoters: some View {
        MonacoRow(
            title: CabalRulesCopy.votersPickTitle, subtitle: CabalRulesCopy.votersPickSubtitle, isLast: true
        ) {
            Image(systemName: "person.3")
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.brand)
                .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
                .background(Circle().fill(MonacoTheme.brandSoft))
                .accessibilityHidden(true)
        }
        .monacoBleed()
        .accessibilityIdentifier("create-rule-voters-pick")
    }

    private var nameProblemMessage: String? {
        guard !form.name.isEmpty else { return nil }
        return form.nameProblem?.message
    }

    private var isHardProblem: Bool {
        !form.name.isEmpty && form.nameProblem?.isHard == true
    }

    private func prepareCover(_ item: PhotosPickerItem) async {
        defer { cover = nil }
        guard let data = try? await item.loadTransferable(type: Data.self) else {
            toasts?.current = MonacoToast(message: "Couldn't read that picture.", isSuccess: false)
            return
        }
        switch await ProfilePhotoUploadPreparer.prepared(from: data) {
        case .success(let prepared):
            coverUpload = prepared
            coverImage = UIImage(data: prepared.data)
        case .failure(let failure):
            toasts?.current = MonacoToast(message: failure.memberMessage, isSuccess: false)
        }
    }

    private func removeCover() {
        coverUpload = nil
        coverImage = nil
    }

    private func create() async {
        guard !isCreating, let input = form.input, let actions else { return }
        isCreating = true
        submitFailed = false
        defer { isCreating = false }
        do {
            let cabal = try await actions.createCabal(input, submission: submission)
            if await uploadCover(for: cabal) {
                toasts?.current = MonacoToast(message: CabalRulesCopy.createdWithoutCover, isSuccess: false)
            } else {
                toasts?.show(success: CabalRulesCopy.created)
            }
            if let onCreated {
                onCreated(cabal)
            } else {
                showCreated(cabal)
            }
            Task { await environment?.pushPrePrompt.noteCabalJoined(after: toasts) }
        } catch {
            submitFailed = true
            toasts?.show(APIError(error))
        }
    }

    private func uploadCover(for cabal: Components.Schemas.Cabal) async -> Bool {
        guard let coverUpload, let environment else { return false }
        let editor = CabalPictureEditor(
            groupId: cabal.id, pictureUrl: nil, writer: LiveCabalPictureWriter(api: environment.api))
        switch await editor.setPicture(imageData: coverUpload.data, mimeType: coverUpload.mimeType) {
        case .saved: return false
        case .failed: return true
        }
    }

    private func showCreated(_ cabal: Components.Schemas.Cabal) {
        guard let navigator = environment?.navigator else { return }
        if navigator.cabalsPath.last == AnyAppRoute(CreateCabalRoute()) {
            navigator.cabalsPath.removeLast()
        }
        navigator.open(CabalRoute(id: cabal.id), in: .cabals)
    }
}

private struct CreateCoverPreview: View {
    let image: UIImage?

    private static let height: CGFloat = 110
    private static let checkSize: CGFloat = 25

    var body: some View {
        ZStack(alignment: .bottomTrailing) {
            if let image {
                Color.clear
                    .overlay { Image(uiImage: image).resizable().scaledToFill() }
                    .clipped()
                MonacoTheme.coverOverlay
                Image(systemName: MonacoIcon.check.symbol)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.onBrand)
                    .frame(width: Self.checkSize, height: Self.checkSize)
                    .background(Circle().fill(MonacoTheme.primaryButtonFill))
                    .padding(.trailing, MonacoTheme.Space.sm)
                    .padding(.bottom, MonacoTheme.Space.sm)
            } else {
                MonacoTheme.brandSoft
                VStack(spacing: MonacoTheme.Space.xs) {
                    Image(systemName: "photo")
                        .font(MonacoTheme.Typo.headline)
                    Text(CabalRulesCopy.coverAdd)
                        .font(MonacoTheme.Typo.captionStrong)
                }
                .foregroundStyle(MonacoTheme.brand)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .frame(maxWidth: .infinity)
        .frame(height: Self.height)
        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.search, style: .continuous))
        .contentShape(Rectangle())
    }
}

private struct CreateRuleChoice<Option: Hashable>: View {
    let options: [Option]
    @Binding var selection: Option
    let label: (Option) -> String

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(spacing: MonacoTheme.Space.s) {
                ForEach(options, id: \.self) { option in
                    stacked(option)
                }
            }
        } else {
            MonacoSegmented(options, selection: $selection, label: label)
        }
    }

    private func stacked(_ option: Option) -> some View {
        let isSelected = option == selection
        return Button {
            guard !isSelected else { return }
            Haptics.selection()
            selection = option
        } label: {
            HStack(spacing: MonacoTheme.Space.s) {
                Text(label(option))
                    .font(MonacoTheme.Typo.calloutStrong)
                    .multilineTextAlignment(.leading)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
                if isSelected {
                    Image(systemName: MonacoIcon.check.symbol)
                        .font(MonacoTheme.Typo.calloutStrong)
                        .accessibilityHidden(true)
                }
            }
            .foregroundStyle(isSelected ? MonacoTheme.onBrand : MonacoTheme.primaryText)
            .padding(.horizontal, MonacoTheme.Space.sm)
            .padding(.vertical, MonacoTheme.Space.s)
            .frame(minHeight: 44)
            .background(Capsule().fill(isSelected ? MonacoTheme.brandFill : MonacoTheme.surfaceSunken))
            .contentShape(Capsule())
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(isSelected ? [.isSelected] : [])
    }
}

#if DEBUG
#Preview {
    NavigationStack {
        CreateGroupView(actions: CabalsTabSampleData.Actions())
    }
}
#endif
