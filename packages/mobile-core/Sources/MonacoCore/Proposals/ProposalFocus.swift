public enum ProposalFocus: Hashable, Sendable {
    case none
    case replies
    case reply(commentID: String)
    case anchor(commentID: String)
}
