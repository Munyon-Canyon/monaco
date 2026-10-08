import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalRulesCopy {
    static let screenTitle = "Start a cabal"
    static let namePlaceholder = "Cabal name"
    static let nameHint = "Pick a name your friends will recognize."
    static let sectionTitle = "The rules"
    static let votersTitle = "Who votes"
    static let thresholdTitle = "To pass"
    static let expiryTitle = "Votes stay open"
    static let create = "Create cabal"
    static let creating = "Creating…"
    static let created = "Cabal created."

    static var auditedStrings: [String] {
        [
            screenTitle, namePlaceholder, nameHint, sectionTitle, votersTitle, thresholdTitle, expiryTitle,
            create, creating, created,
        ]
            + CabalVoterMode.allCases.flatMap { [$0.label, $0.caption] }
            + CabalThreshold.allCases.flatMap { [$0.label, $0.caption] }
            + CabalProposalExpiry.allCases.flatMap { [$0.label, $0.caption] }
            + [CreateCabalForm.NameProblem.tooShort, .tooLong, .invalid].compactMap(\.message)
    }
}

struct CreateGroupView: View {
    @Environment(AppEnvironment.self) private var environment: AppEnvironment?
    @Environment(ToastCenter.self) private var toasts: ToastCenter?

    private let injectedActions: CabalsActionSource?
    private let onCreated: ((Components.Schemas.Cabal) -> Void)?

    @State private var form = CreateCabalForm()
    @State private var submission = IdempotentSubmission()
    @State private var isCreating = false

    init(
        actions: CabalsActionSource? = nil,
        onCreated: ((Components.Schemas.Cabal) -> Void)? = nil
    ) {
        self.injectedActions = actions
        self.onCreated = onCreated
    }

    private var actions: CabalsActionSource? {
        if let injectedActions { return injectedActions }
        guard let environment else { return nil }
        return LiveCabalsActionSource(auth: environment.auth, api: environment.api)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                nameField
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                CabalRulesSection(
                    voterSet: $form.voterMode,
                    threshold: $form.threshold,
                    voteExpiry: $form.expiry,
                    identifierPrefix: "create-rule"
                )
                .disabled(isCreating)
            }
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .scrollDismissesKeyboard(.interactively)
        .monacoCanvas()
        .navigationTitle(CabalRulesCopy.screenTitle)
        .navigationBarTitleDisplayMode(.large)
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button {
                    Task { await create() }
                } label: {
                    HStack(spacing: MonacoTheme.Space.s) {
                        if isCreating {
                            ProgressView().tint(MonacoTheme.primaryButtonLabel)
                            Text(CabalRulesCopy.creating)
                        } else {
                            Text(CabalRulesCopy.create)
                        }
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(.monacoPrimary)
                .disabled(isCreating || form.input == nil || actions == nil)
                .accessibilityIdentifier("create-group-submit")
            }
        }
        .onChange(of: form) { _, _ in
            submission = IdempotentSubmission()
        }
    }

    private var nameField: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoTextField(
                CabalRulesCopy.namePlaceholder,
                text: $form.name,
                isInvalid: nameProblemMessage != nil,
                isOutlined: true,
                errorMessage: nameProblemMessage
            )
            .submitLabel(.done)
            .disabled(isCreating)
            .accessibilityIdentifier("create-group-name")
            if let problem = nameProblemMessage {
                Text(problem)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.loss)
                    .accessibilityIdentifier("create-group-name-problem")
            } else {
                Text(CabalRulesCopy.nameHint)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
        }
    }

    private var nameProblemMessage: String? {
        guard !form.name.isEmpty else { return nil }
        return form.nameProblem?.message
    }

    private func create() async {
        guard !isCreating, let input = form.input, let actions else { return }
        isCreating = true
        defer { isCreating = false }
        do {
            let cabal = try await actions.createCabal(input, submission: submission)
            toasts?.show(success: CabalRulesCopy.created)
            if let onCreated {
                onCreated(cabal)
            } else {
                showCreated(cabal)
            }
            Task { await environment?.pushPrePrompt.noteCabalJoined(after: toasts) }
        } catch {
            toasts?.show(APIError(error))
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

#if DEBUG
#Preview {
    NavigationStack {
        CreateGroupView(actions: CabalsTabSampleData.Actions())
    }
}
#endif
