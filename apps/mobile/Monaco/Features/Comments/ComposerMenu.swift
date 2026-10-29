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
    @State private var sharing = false
    @Environment(ToastCenter.self) private var toasts
    @Environment(AppEnvironment.self) private var appEnvironment: AppEnvironment?
    @Environment(\.stockSearch) private var stockSearch

    private var search: StockSearch? { stockSearch ?? appEnvironment.map { LiveStockSearch.make($0.api) } }

    private var quotable: String? { ThesisQuote.isQuotable(thesis) && model != nil ? thesis : nil }
    var onPoll: (() -> Void)?

    private var available: [ComposerMenuItem] { items.filter { $0.isAvailable(target) } }

    var body: some View {
        if onPoll != nil || !available.isEmpty || quotable != nil
            || (model != nil && search != nil)
        {
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
                if model != nil, search != nil {
                    Button {
                        sharing = true
                    } label: {
                        Label("Share a stock", monacoIcon: .markets)
                    }
                    .accessibilityIdentifier("composer-menu-stock")
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
            .sheet(isPresented: $sharing) {
                if let search, let model {
                    let toasts = toasts
                    StockShareSheet(search: search) { model.pick($0, toasts: toasts) }
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
