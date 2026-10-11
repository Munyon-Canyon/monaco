#if DEBUG
import Foundation

extension Components.Schemas.BlockedUser {
    public static let sample = Self(
        userId: "01890a5d-ac96-774b-bcce-b302099a8061", handle: "qa_b", displayName: "QA Bea", photoUrl: nil)

    public static let sampleSecond = Self(
        userId: "01890a5d-ac96-774b-bcce-b302099a8062", handle: "kai", displayName: "Kai Cenat", photoUrl: nil)
}

extension Components.Schemas.BlockedUsers {
    public static let sample = Self(users: [.sample, .sampleSecond])

    public static let sampleEmpty = Self(users: [])
}
#endif
