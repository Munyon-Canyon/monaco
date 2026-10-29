public struct PushActionSpec: Equatable, Sendable {
    public let id: String
    public let title: String
    public let symbol: String
    public let foreground: Bool
    public let authenticationRequired: Bool
}

public struct PushCategory: Equatable, Sendable {
    public let id: String
    public let actions: [PushActionSpec]

    private static let yes = PushActionSpec(
        id: PushAction.voteYes, title: "Yes", symbol: "checkmark", foreground: true, authenticationRequired: true)
    private static let no = PushActionSpec(
        id: PushAction.voteNo, title: "No", symbol: "xmark", foreground: true, authenticationRequired: true)
    private static let thesis = PushActionSpec(
        id: PushAction.readThesis, title: "Read the thesis", symbol: "bubble.left", foreground: true,
        authenticationRequired: false)

    public static let all: [PushCategory] = [
        PushCategory(id: "VOTE", actions: [yes, no, thesis]),
        PushCategory(id: "VOTE_DECIDING", actions: [yes, no]),
        PushCategory(id: "LEDGER", actions: []),
    ]
}
