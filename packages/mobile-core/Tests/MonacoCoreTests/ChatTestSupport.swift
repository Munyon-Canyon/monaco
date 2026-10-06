import Foundation
import MonacoAPI
import MonacoCore

enum ChatFixtures {
    static let cabalID = "01920000-0000-7000-8000-000000000001"
    static let viewerID = "01890a5d-ac96-774b-bcce-b302099a8058"
    static let otherID = "01890a5d-ac96-774b-bcce-b302099a8061"
    static let epoch = Date(timeIntervalSince1970: 1_790_000_000)

    static func message(
        _ id: String,
        author: String = otherID,
        body: String? = nil,
        minutes: Double = 0,
        parentID: String? = nil,
        alsoInChannel: Bool = false,
        replyCount: Int = 0,
        seenCount: Int? = nil
    ) -> ChatMessage {
        ChatMessage(
            id: id,
            author: .init(id: author, handle: nil, displayName: author == viewerID ? "Me" : "Kai", photoUrl: nil),
            body: body ?? "text \(id)",
            createdAt: epoch.addingTimeInterval(minutes * 60),
            parentId: parentID,
            alsoInChannel: alsoInChannel,
            replyCount: replyCount,
            deleted: false,
            seenCount: seenCount
        )
    }

    static func json(_ message: ChatMessage) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(message), as: UTF8.self)
    }
}
