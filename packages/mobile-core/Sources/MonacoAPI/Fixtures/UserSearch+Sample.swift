#if DEBUG
extension Components.Schemas.UserSummary {
    public static let samples: [Self] = [
        Self(
            userId: "01890a5d-ac96-774b-bcce-b302099a8058", handle: "maya", displayName: "Maya Angelou",
            photoUrl: "https://cdn.example.com/photos/maya.jpg", followedByMe: true),
        Self(
            userId: "01890a5d-ac96-774b-bcce-b302099a8059", handle: "mayor", displayName: "", photoUrl: nil,
            followedByMe: false),
        Self(
            userId: "01890a5d-ac96-774b-bcce-b302099a805a", handle: "qa_b", displayName: "Bea", photoUrl: nil,
            followedByMe: false),
    ]
}
#endif
