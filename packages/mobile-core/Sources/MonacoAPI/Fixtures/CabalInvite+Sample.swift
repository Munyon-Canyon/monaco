#if DEBUG
import Foundation

extension Components.Schemas.CabalInvite {
    public static let sample = Self(
        requestId: "00000000-0000-7000-8000-00000000e001",
        cabal: .init(
            id: "00000000-0000-7000-8000-00000000c001", name: "Friday Fund", pictureUrl: nil, memberCount: 4
        ),
        invitedBy: .init(
            userId: "00000000-0000-7000-8000-00000000a001", handle: "kaicenat", displayName: "Kai Cenat",
            photoUrl: nil
        ),
        expiresAt: Date(timeIntervalSince1970: 1_791_590_400)
    )

    public static let sampleSolo = Self(
        requestId: "00000000-0000-7000-8000-00000000e002",
        cabal: .init(
            id: "00000000-0000-7000-8000-00000000c002", name: "Long Haul", pictureUrl: nil, memberCount: 1
        ),
        invitedBy: .init(
            userId: "00000000-0000-7000-8000-00000000a002", handle: nil, displayName: "Mara", photoUrl: nil
        ),
        expiresAt: Date(timeIntervalSince1970: 1_791_590_400)
    )

    public static let samples: [Self] = [.sample, .sampleSolo]
}
#endif
