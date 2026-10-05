#if DEBUG
import Foundation

extension Components.Schemas.OnrampSessionCreated {
    public static let sample = Self(
        sessionId: "01890a5d-ac96-774b-bcce-b302099a8057",
        url: "https://monacolabs.xyz/fund?s=q2J9cZQxv0mYb5r8yS3dTt1uVw7xY9zA0bC2dE4fG6h",
        expiresAt: Date(timeIntervalSince1970: 1_759_579_800)
    )
}

extension Components.Schemas.OnrampSession {
    public static let sample = Self(
        sessionId: "01890a5d-ac96-774b-bcce-b302099a8057",
        status: .confirmed,
        suggestedAmountMicros: "25000000",
        createdAt: Date(timeIntervalSince1970: 1_759_579_200),
        completedAt: Date(timeIntervalSince1970: 1_759_579_440)
    )
}
#endif
