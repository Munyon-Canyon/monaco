#if DEBUG
import Foundation

extension Components.Schemas.CabalAccessRequest {
    public static let samples: [Self] = [
        Self(
            id: "00000000-0000-7000-8000-00000000e011",
            user: .init(
                userId: "00000000-0000-7000-8000-00000000a011", handle: "leo", displayName: "Leo", photoUrl: nil),
            via: .init(
                userId: "00000000-0000-7000-8000-00000000a013", handle: "jordan", displayName: "Jordan", photoUrl: nil),
            createdAt: Date(timeIntervalSinceNow: -2 * 60 * 60)
        ),
        Self(
            id: "00000000-0000-7000-8000-00000000e012",
            user: .init(
                userId: "00000000-0000-7000-8000-00000000a012", handle: "priya", displayName: "", photoUrl: nil),
            via: nil,
            createdAt: Date(timeIntervalSince1970: 1_790_000_600)
        ),
    ]
}
#endif
