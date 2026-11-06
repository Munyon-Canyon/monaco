import MonacoAPI
import SwiftUI
import Testing

@testable import Monaco
@testable import MonacoCore

@MainActor
struct ChatAttachmentRowTests {
    private let viewer = "u-me"

    private func message(
        _ id: String = "m1", body: String = "GIF", attachment: Components.Schemas.CommentAttachment? = nil,
        deleted: Bool = false
    ) -> ChatMessage {
        var message = ChatMessage(
            id: id, kind: .user,
            author: .init(id: "u-sam", handle: nil, displayName: "Sam", photoUrl: nil),
            body: body,
            createdAt: Date(timeIntervalSinceNow: -60),
            alsoInChannel: false,
            replyCount: 0,
            lastRepliers: [], deleted: deleted,
            reactions: [],
            reactionCount: 0
        )
        message.attachment = attachment
        return message
    }

    private func row(_ message: ChatMessage) -> ChatRow {
        var timeline = ChatTimeline(viewerID: viewer)
        timeline.mergeNewest([message], pageSize: 50)
        return timeline.rows[0]
    }

    private func pendingRow(_ draft: ChatAttachmentDraft, body: String? = nil) -> ChatRow {
        var timeline = ChatTimeline(viewerID: viewer)
        timeline.addUnsent(key: "k1", body: body, attachment: draft, at: Date(timeIntervalSinceNow: -5))
        return timeline.rows[0]
    }

    private var gifDraft: ChatAttachmentDraft {
        let media = Components.Schemas.GifMedia(url: "https://media.giphy.com/media/x/200.gif", width: 200, height: 100)
        return .gif(.init(id: "x", title: "Clap", still: media, preview: media, original: media))
    }

    @Test func aPlainMessageIsText() async {
        #expect(ChatAttachmentRow.content(of: row(message(body: "gm"))) == .text)
    }

    @Test func eachSentKindMapsToItsContent() async {
        let gif = row(message(attachment: .gif(.sample())))
        let poll = row(message("m2", attachment: .poll(.sample())))
        #expect(ChatAttachmentRow.content(of: gif) == .attachment(.gif))
        #expect(ChatAttachmentRow.content(of: poll) == .attachment(.poll))
        let stock = Components.Schemas.StockAttachment(
            kind: .stock, assetId: "a", symbol: "AAPL", name: "Apple", logoUrl: "https://example.com/a.png",
            isPreIpo: false, priceMicros: nil, changeBps: nil)
        let quote = Components.Schemas.ThesisQuoteAttachment(
            kind: .thesisQuote, proposalId: "p", text: "Small add.", proposerName: "Mia")
        #expect(ChatAttachmentRow.kind(of: .stock(stock)) == .stock)
        #expect(ChatAttachmentRow.kind(of: .thesisQuote(quote)) == .thesis)
    }

    @Test func aMessageWithTextShowsItAboveTheAttachment() async {
        let withText = row(message(body: "look at this", attachment: .gif(.sample())))
        let bare = row(message("m2", attachment: .gif(.sample())))
        #expect(ChatAttachmentRow.showsTextAbove(withText))
        #expect(!ChatAttachmentRow.showsTextAbove(bare))
    }

    @Test func aPendingAttachmentIsASkeletonOfItsKind() async {
        let pending = pendingRow(gifDraft)
        #expect(ChatAttachmentRow.content(of: pending) == .pending(.gif))
        #expect(ChatAttachmentRow.content(of: pendingRow(.stock(assetID: "a"))) == .pending(.stock))
        #expect(
            ChatAttachmentRow.content(of: pendingRow(.thesisQuote(proposalID: "p", start: nil, end: nil)))
                == .pending(.thesis))
        #expect(ChatAttachmentRow.skeletonSize(gifDraft).width == ChatAttachmentRow.gifWidth)
        #expect(ChatAttachmentRow.skeletonSize(gifDraft).height == ChatAttachmentRow.gifWidth / 2)
        #expect(ChatAttachmentRow.skeletonSize(.stock(assetID: "a")).width == GifBubbleMetrics.width)
    }

    @Test func aDeletedMessageStaysDeleted() async {
        let deleted = row(message(attachment: .gif(.sample()), deleted: true))
        #expect(ChatAttachmentRow.content(of: deleted) == .deleted)
        #expect(!ChatAttachmentRow.showsAttachment(deleted))
    }

    @Test func theGifLabelNamesTheSenderAndTitle() async {
        #expect(ChatAttachmentRow.gifLabel(author: "Sam", isMine: false, title: "Clap") == "GIF from Sam, Clap")
        #expect(ChatAttachmentRow.gifLabel(author: "Sam", isMine: false, title: "") == "GIF from Sam")
        #expect(ChatAttachmentRow.gifLabel(author: "Me", isMine: true, title: "Clap") == "GIF from You, Clap")
    }
}
