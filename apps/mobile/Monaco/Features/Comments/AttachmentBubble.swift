import MonacoAPI
import SwiftUI

struct AttachmentBubble: View {
    let commentID: String
    let attachment: Components.Schemas.CommentAttachment

    var body: some View {
        switch attachment {
        case .gif(let gif): GifBubble(commentID: commentID, attachment: gif)
        default: EmptyView()
        }
    }
}
