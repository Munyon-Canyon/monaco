import MonacoAPI
import MonacoCore
import SwiftUI

struct PollComposerSheet: View {
    let isCabal: Bool
    let onPost: (Components.Schemas.CommentAttachmentInput) -> Void

    @State private var draft = CommentPollDraft()
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            MonacoTextField("Question", text: $draft.question, autofocus: true)
                .accessibilityIdentifier("poll-question-field")
            ForEach(draft.options.indices, id: \.self) { index in
                MonacoTextField("Option \(index + 1)", text: $draft.options[index])
                    .accessibilityIdentifier("poll-option-field-\(index + 1)")
            }
            if draft.canAddOption {
                Button("Add an option") { draft.addOption() }
                    .buttonStyle(.monacoCompact)
                    .accessibilityIdentifier("poll-add-option")
            }
            closesRow
            Button("Post") {
                guard let input = draft.input else { return }
                onPost(input)
                dismiss()
            }
            .buttonStyle(.monacoPrimary)
            .disabled(!draft.isValid)
            .accessibilityIdentifier("poll-post")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.s)
        .monacoFittedSheet(
            title: CommentsCopy.pollTitle(isCabal: isCabal), titleIdentifier: "poll-title", detents: .fitted,
            showsClose: true, closeIdentifier: "poll-close"
        )
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("poll-sheet")
    }

    private var closesRow: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text("Closes in")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            ForEach(CommentPollDraft.closeChoices, id: \.self) { choice in
                Button(choice.label) { draft.closes = choice }
                    .font(draft.closes == choice ? MonacoTheme.Typo.captionStrong : MonacoTheme.Typo.caption)
                    .foregroundStyle(draft.closes == choice ? MonacoTheme.brand : MonacoTheme.secondaryText)
                    .frame(minWidth: CommentRowMetrics.replyTarget, minHeight: CommentRowMetrics.replyTarget)
                    .accessibilityAddTraits(draft.closes == choice ? .isSelected : [])
                    .accessibilityIdentifier("poll-closes-\(choice.label)")
            }
        }
    }
}

struct PollComposer: ViewModifier {
    @Binding var isPresented: Bool
    let model: CommentsModel
    let isCabal: Bool

    func body(content: Content) -> some View {
        content.sheet(isPresented: $isPresented) {
            PollComposerSheet(isCabal: isCabal) { input in
                Task { _ = await model.post(CommentDraft(text: model.draft), attachment: input) }
            }
        }
    }
}

extension View {
    func pollComposer(isPresented: Binding<Bool>, model: CommentsModel, isCabal: Bool) -> some View {
        modifier(PollComposer(isPresented: isPresented, model: model, isCabal: isCabal))
    }
}
