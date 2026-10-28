#if DEBUG
import Foundation

extension Components.Schemas.MyReferralCode {
    public static let sample = Self(
        code: "k7m4qx2p", link: "https://monacolabs.xyz/r/k7m4qx2p",
        handleLink: "https://monacolabs.xyz/r/kaicenat", handleUnlocked: true)

    public static let sampleLocked = Self(
        code: "k7m4qx2p", link: "https://monacolabs.xyz/r/k7m4qx2p", handleLink: nil, handleUnlocked: false)
}

extension Components.Schemas.ReferralCabal {
    private typealias Person = Components.Schemas.ReferralCabalPerson

    private static let andres = InviterPayload(displayName: "Andres", photoUrl: "https://img.example/andres.png")

    private static let acCapital = CabalPayload(
        id: "01890a5d-ac96-774b-bcce-b302099a8058", name: "AC Capital", pictureUrl: nil, memberCount: 8,
        since: "2026-08-14", joinMode: .request,
        firstMembers: [
            Person(displayName: "Christelle", photoUrl: "https://img.example/christelle.png"),
            Person(displayName: "Logan", photoUrl: "https://img.example/logan.png"),
            Person(displayName: "Mia", photoUrl: "https://img.example/mia.png"),
        ],
        potValueMicros: "20000000000")

    private static let nvdaProposal = OpenProposalPayload(
        id: "01890a5d-ac96-774b-bcce-b302099a8057", kind: "buy", symbol: "NVDAx", assetName: "NVIDIA",
        assetLogoUrl: nil, usdcMicros: "400000000", proposerName: "Mia", title: "small add before earnings",
        yes: 4, no: 1, voted: 5, eligible: 8, replies: 6, expiresAt: Date(timeIntervalSince1970: 1_793_200_000))

    public static let sample = Self(
        inviter: andres, cabal: acCapital, return1mBps: -410, turnoutMonthBps: 9400, openProposal: nvdaProposal)

    public static let sampleMonthFactsNull = Self(
        inviter: andres, cabal: acCapital, return1mBps: nil, turnoutMonthBps: nil, openProposal: nil)

    public static let sampleNoOpenProposal = Self(
        inviter: andres, cabal: acCapital, return1mBps: -410, turnoutMonthBps: 9400, openProposal: nil)

    public static let sampleMiss = Self(
        inviter: nil, cabal: nil, return1mBps: nil, turnoutMonthBps: nil, openProposal: nil)
}
#endif
