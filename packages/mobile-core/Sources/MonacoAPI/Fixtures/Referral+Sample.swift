#if DEBUG
extension Components.Schemas.MyReferralCode {
    public static let sample = Self(
        code: "k7m4qx2p", link: "https://monacolabs.xyz/r/k7m4qx2p",
        handleLink: "https://monacolabs.xyz/r/kaicenat", handleUnlocked: true)

    public static let sampleLocked = Self(
        code: "k7m4qx2p", link: "https://monacolabs.xyz/r/k7m4qx2p", handleLink: nil, handleUnlocked: false)
}
#endif
