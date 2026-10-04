nonisolated struct UserPreview: Sendable, Hashable {
    let displayName: String
    let handle: String?
    let photoURL: String?
}

nonisolated struct UserProfileContext: Sendable, Hashable {
    let userID: String
    let preview: UserPreview?

    init(userID: String, preview: UserPreview? = nil) {
        self.userID = userID
        self.preview = preview
    }
}
