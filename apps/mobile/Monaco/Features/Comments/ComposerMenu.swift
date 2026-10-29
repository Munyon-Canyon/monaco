import MonacoCore
import SwiftUI

struct ComposerMenuItem: Identifiable {
    let id: String
    let title: String
    let icon: MonacoIcon
    let isAvailable: (ReplyTarget) -> Bool
    let action: @MainActor () -> Void
}

struct ComposerMenu: View {
    let items: [ComposerMenuItem]
    let target: ReplyTarget
    var thesis: String?
    var model: CommentsModel?

    @State private var quoting = false
    @Environment(ToastCenter.self) private var toasts

    private var quotable: String? { ThesisQuote.isQuotable(thesis) && model != nil ? thesis : nil }
    var onPoll: (() -> Void)?

    private var available: [ComposerMenuItem] { items.filter { $0.isAvailable(target) } }

    var body: some View {
        if onPoll != nil || !available.isEmpty || quotable != nil {
            Menu {
                if let onPoll {
                    Button(action: onPoll) {
                        Label(CommentsCopy.pollMenu, monacoIcon: .pollCheck)
                    }
                    .accessibilityIdentifier("composer-menu-poll")
                }
                if quotable != nil {
                    Button {
                        quoting = true
                    } label: {
                        Label("Quote the thesis", monacoIcon: .thesis)
                    }
                    .accessibilityIdentifier("composer-menu-thesis")
                }
                ForEach(available) { item in
                    Button(action: item.action) {
                        Label(item.title, monacoIcon: item.icon)
                    }
                    .accessibilityIdentifier("composer-menu-\(item.id)")
                }
            } label: {
                Image(monaco: .plusCircle)
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.brand)
                    .frame(width: CommentRowMetrics.replyTarget, height: CommentRowMetrics.replyTarget)
                    .contentShape(Rectangle())
            }
            .sheet(isPresented: $quoting) {
                if let thesis = quotable, let model {
                    let toasts = toasts
                    ThesisQuoteSheet(thesis: thesis) { model.pick($0, toasts: toasts) }
                }
            }
            .menuOrder(.fixed)
            .accessibilityLabel(CommentsCopy.plusAccessibility)
            .accessibilityIdentifier("comment-composer-plus")
        }
    }
}

extension Label where Title == Text, Icon == Image {
    fileprivate init(_ title: String, monacoIcon: MonacoIcon) {
        self.init(
            title: { Text(title) },
            icon: { Image(monaco: monacoIcon) })
    }
}
