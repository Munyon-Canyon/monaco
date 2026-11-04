import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatThesisSource: Equatable {
    let proposalID: String
    let text: String

    static func newest(in proposals: [ProposalSummary]) -> ChatThesisSource? {
        proposals
            .filter { $0.status == .open && !$0.isRule && ThesisQuote.isQuotable($0.thesis) }
            .max { $0.createdAt < $1.createdAt }
            .flatMap { proposal in proposal.thesis.map { ChatThesisSource(proposalID: proposal.id, text: $0) } }
    }
}

struct ChatComposerAttachments {
    let gifs: any GifProvider
    let search: StockSearch?
    let thesis: ChatThesisSource?
    let send: (String?, ChatAttachmentDraft) -> Void
}

enum ChatComposerItem: String, CaseIterable, Identifiable {
    case gif
    case thesis
    case stock

    var id: String { rawValue }

    var title: String {
        switch self {
        case .gif: GroupChatCopy.menuGif
        case .thesis: GroupChatCopy.menuThesis
        case .stock: GroupChatCopy.menuStock
        }
    }

    var icon: MonacoIcon {
        switch self {
        case .gif: .gif
        case .thesis: .thesis
        case .stock: .markets
        }
    }

    var identifier: String { "chat-menu-\(rawValue)" }
}

struct ChatComposerMenu: View {
    let hasThesis: Bool
    var hasStock = true
    let pick: (ChatComposerItem) -> Void

    static func items(hasThesis: Bool, hasStock: Bool = true) -> [ChatComposerItem] {
        ChatComposerItem.allCases.filter {
            switch $0 {
            case .gif: true
            case .thesis: hasThesis
            case .stock: hasStock
            }
        }
    }

    var body: some View {
        Menu {
            ForEach(Self.items(hasThesis: hasThesis, hasStock: hasStock)) { item in
                Button {
                    pick(item)
                } label: {
                    Label {
                        Text(item.title)
                    } icon: {
                        Image(monaco: item.icon)
                    }
                }
                .accessibilityIdentifier(item.identifier)
            }
        } label: {
            Image(monaco: .plusCircle)
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
                .frame(width: MonacoButtonMetrics.hitTarget, height: MonacoButtonMetrics.hitTarget)
                .contentShape(Rectangle())
        }
        .menuOrder(.fixed)
        .accessibilityLabel(GroupChatCopy.addToMessage)
        .accessibilityIdentifier("chat-composer-plus")
    }
}

struct ChatComposerSheets: ViewModifier {
    @Binding var active: ChatComposerItem?
    let attachments: ChatComposerAttachments?
    let send: (ChatAttachmentDraft) -> Void

    func body(content: Content) -> some View {
        content.sheet(item: $active) { item in
            if let attachments {
                sheet(for: item, attachments)
            }
        }
    }

    @ViewBuilder private func sheet(for item: ChatComposerItem, _ attachments: ChatComposerAttachments) -> some View {
        switch item {
        case .gif:
            GifSheet(gifs: attachments.gifs) { deliver($0, nil) }
        case .stock:
            if let search = attachments.search {
                StockShareSheet(search: search) { deliver($0.input, nil) }
            }
        case .thesis:
            if let thesis = attachments.thesis {
                ThesisQuoteSheet(thesis: thesis.text) { deliver($0.input, thesis.proposalID) }
            }
        }
    }

    private func deliver(_ input: Components.Schemas.CommentAttachmentInput, _ proposalID: String?) {
        guard let draft = ChatAttachmentDraft(input, proposalID: proposalID) else { return }
        send(draft)
    }
}
