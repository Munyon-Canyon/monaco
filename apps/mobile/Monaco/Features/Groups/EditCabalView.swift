import MonacoAPI
import MonacoCore
import SwiftUI

enum EditCabalCopy {
    static let screenTitle = "Cabal settings"
    static let save = "Save"
    static let saving = "Saving…"
    static let saved = "Cabal updated."
    static let votersHeader = "Voters"
    static let alwaysVotes = "Always votes"

    static var auditedStrings: [String] {
        [screenTitle, save, saving, saved, votersHeader, alwaysVotes]
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
}
