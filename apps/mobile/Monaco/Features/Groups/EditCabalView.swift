import MonacoAPI
import MonacoCore
import SwiftUI

enum EditCabalCopy {
    static let screenTitle = "Cabal settings"
    static let save = "Save"
    static let saving = "Saving…"
    static let saved = "Cabal updated."
    static let rulesFooter = "Vote rules apply to new proposals. Who can join applies right away."
    static let votersHeader = "Voters"
    static let alwaysVotes = "Always votes"

    static var auditedStrings: [String] {
        [screenTitle, save, saving, saved, rulesFooter, votersHeader, alwaysVotes]
    }
}

struct EditCabalView: View {
    let model: CabalEditModel
    let cabal: Components.Schemas.Cabal

    @StateObject private var pictureEditor: CabalPictureEditor
    @State private var edited: CabalSettings
    @Environment(ToastCenter.self) private var toasts: ToastCenter?
    @Environment(\.dismiss) private var dismiss

    init(model: CabalEditModel, cabal: Components.Schemas.Cabal, pictureWriter: any CabalPictureWriting) {
        self.model = model
        self.cabal = cabal
        _edited = State(initialValue: CabalSettings(cabal))
        _pictureEditor = StateObject(
            wrappedValue: CabalPictureEditor(groupId: cabal.id, pictureUrl: cabal.pictureUrl, writer: pictureWriter)
        )
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                CabalPicturePicker(
                    groupId: cabal.id,
                    name: edited.name,
                    canEdit: true,
                    size: 88,
                    onResult: { toasts?.current = $0 },
                    editor: pictureEditor
                )
                .frame(maxWidth: .infinity)
                nameField
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    CabalRulesSection(
                        joinPolicy: $edited.joinPolicy,
                        voterSet: voterSet,
                        threshold: threshold,
                        voteExpiry: voteExpiry,
                        identifierPrefix: "edit-rule",
                        showsThreshold: Self.showsThreshold(memberCount: Int((model.cabal ?? cabal).memberCount))
                    )
                    Text(EditCabalCopy.rulesFooter)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                        .accessibilityIdentifier("edit-cabal-rules-footer")
                }
                .disabled(model.isSaving)
                if edited.voters != .everyone {
                    CabalVoterChecklist(
                        cabal: model.cabal ?? cabal, voters: $edited.voters, isDisabled: model.isSaving)
                }
            }
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .scrollDismissesKeyboard(.interactively)
        .monacoCanvas()
        .navigationTitle(EditCabalCopy.screenTitle)
        .navigationBarTitleDisplayMode(.inline)
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button {
                    Task { await save() }
                } label: {
                    SubmitLabel(isWorking: model.isSaving, idle: EditCabalCopy.save, working: EditCabalCopy.saving)
                }
                .buttonStyle(.monacoPrimary)
                .disabled(!canSave)
                .accessibilityIdentifier("edit-cabal-save")
            }
        }
    }

    private var nameField: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoTextField(
                CabalRulesCopy.namePlaceholder,
                text: $edited.name,
                isInvalid: CreateCabalForm(name: edited.name).nameProblem?.isHard == true,
                capitalization: .words,
                autocorrects: false
            )
            .submitLabel(.done)
            .disabled(model.isSaving)
            .accessibilityIdentifier("edit-cabal-name")
            if let problem = nameProblemMessage {
                Text(problem)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.loss)
                    .accessibilityIdentifier("edit-cabal-name-problem")
            }
        }
    }

    private var nameProblemMessage: String? {
        CreateCabalForm(name: edited.name).nameProblem?.message
    }

    static func showsThreshold(memberCount: Int) -> Bool { memberCount > 1 }

    private var canSave: Bool {
        guard !model.isSaving, CreateCabalForm(name: edited.name).nameProblem == nil else {
            return false
        }
        return model.patch(for: edited)?.isEmpty == false
    }

    private func save() async {
        switch await model.save(edited) {
        case .saved:
            if let saved = model.settings { edited = saved }
            toasts?.show(success: EditCabalCopy.saved)
            dismiss()
        case .unchanged:
            break
        case .failed(let message):
            toasts?.current = MonacoToast(message: message)
        }
    }

    private var voterSet: Binding<CabalVoterMode> {
        Binding(
            get: { edited.voters == .everyone ? .everyone : .picked },
            set: { mode in
                switch mode {
                case .everyone: edited.voters = .everyone
                case .picked: if edited.voters == .everyone { edited.voters = .list([]) }
                }
            }
        )
    }

    private var threshold: Binding<CabalThreshold> {
        Binding(
            get: { CabalThreshold(rawValue: edited.threshold) ?? .majority },
            set: { edited.threshold = $0.rawValue }
        )
    }

    private var voteExpiry: Binding<CabalProposalExpiry> {
        Binding(
            get: { CabalProposalExpiry(rawValue: edited.proposalExpirySeconds) ?? CreateCabalForm.defaultExpiry },
            set: { edited.proposalExpirySeconds = $0.rawValue }
        )
    }
}
