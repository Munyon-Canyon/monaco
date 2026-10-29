import MonacoAPI
import SwiftUI

struct AttachmentBubble: View {
    let commentID: String
    let attachment: Components.Schemas.CommentAttachment

    var body: some View {
        switch attachment {
        case .gif(let gif): GifBubble(commentID: commentID, attachment: gif)
        case .thesisQuote(let quote): ThesisQuoteBubble(commentID: commentID, attachment: quote)
        case .stock(let stock): StockShareBubble(commentID: commentID, stock: stock)
        case .poll(let poll): PollBubble(commentID: commentID, poll: poll)
        }
    }
}
