import MonacoCore
import SwiftUI

struct ReplySheet: View {
    let model: CommentsModel
    let target: ReplyTarget
    var owner: String?
    var menuItems: [ComposerMenuItem] = []
    var thesis: String?
    var offersPoll = false
    var asksCabal = false

    @FocusState private var focused: Bool
    @State private var picksGif = false
    @State private var picksPoll = false
    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        let toasts = toasts
        return VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            ReplySheetQuote(target: target)
            ComposerNotices(model: model)
            ComposerAttachmentSlot(model: model)
            HStack(alignment: .bottom, spacing: MonacoTheme.Space.s) {
                if model.draft.isEmpty {
                    ComposerMenu(
                        items: menuItems, target: target, thesis: thesis, model: model,
                        onPoll: offersPoll ? { picksPoll = true } : nil)
                }
                ComposerField(
                    model: model, focused: $focused,
                    onGif: {
                        focused = false
                        picksGif = true
                    })
                ComposerSendButton(model: model) {
                    if case .post = target { toasts.show(success: CommentsCopy.replied) }
                }
            }
            ComposerTooLong(draft: CommentDraft(text: model.draft))
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.s)
        .monacoFittedSheet(
            title: title, titleIdentifier: "reply-sheet-title", detents: .fitted, showsClose: true,
            closeIdentifier: "reply-sheet-close"
        )
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("reply-sheet")
        .gifPicker(isPresented: $picksGif, model: model)
        .pollComposer(isPresented: $picksPoll, model: model, isCabal: asksCabal)
        .onAppear { focused = true }
    }

    private var title: String {
        switch target {
        case .comment(let row): CommentsCopy.replyTitle(row.authorName)
        case .post(let item): CommentsCopy.replyTitle(ReplySheetQuote.name(of: item))
        case .thread: owner.map(CommentsCopy.replyTitle) ?? CommentsCopy.replyTitleBare
        }
    }
}
