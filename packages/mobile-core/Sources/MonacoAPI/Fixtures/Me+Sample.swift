#if DEBUG
import Foundation

extension Components.Schemas.Me {
    public static let sample = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8058", handle: "kaicenat",
        displayName: "Kai Cenat", photoUrl: "https://cdn.example.com/photos/kai.jpg",
        authState: .onboardingCompleted, accountStatus: .active,
        memberWalletAddress: "wallet-1", phoneLinked: true, xUsername: "kaicenat",
        handleChangeableAt: Date(timeIntervalSince1970: 1_793_347_200),
        createdAt: Date(timeIntervalSince1970: 1_759_233_600)
    )
}
#endif
