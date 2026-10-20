import MonacoAPI

struct FeedLikeService: Sendable {
    let api: APIClient

    func set(_ liked: Bool, itemID: String) async throws {
        try await api.submit(
            IdempotentSubmission(), payload: itemID, operation: liked ? "putFeedLike" : "deleteFeedLike"
        ) { client, key in
            if liked {
                _ = try await client.putFeedLike(path: .init(id: itemID), headers: .init(idempotencyKey: key))
                    .noContent
            } else {
                _ = try await client.deleteFeedLike(path: .init(id: itemID), headers: .init(idempotencyKey: key))
                    .noContent
            }
        }
    }
}

extension Components.Schemas.FeedItem {
    mutating func setLike(_ liked: Bool, from before: Self) {
        guard liked != before.likedByMe else { return }
        likedByMe = liked
        likeCount = max(0, before.likeCount + (liked ? 1 : -1))
    }
}
