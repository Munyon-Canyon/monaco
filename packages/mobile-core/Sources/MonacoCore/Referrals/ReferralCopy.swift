public enum ReferralCopy {
    public static let invalidCode = "That code isn't valid."
    public static let invalidLink = "That link isn't a Monaco invite."
    public static let inviteAdded = "Invite added."

    public static func joined(handle: String) -> String {
        "You joined from @\(handle)'s invite."
    }
}
