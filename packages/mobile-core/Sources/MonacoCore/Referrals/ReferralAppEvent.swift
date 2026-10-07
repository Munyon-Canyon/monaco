public enum ReferralAppEvent: String, Sendable, CaseIterable {
    case invitePasteShown = "invite_paste_shown"
    case invitePasted = "invite_pasted"
    case inviteSkipped = "invite_skipped"
}
