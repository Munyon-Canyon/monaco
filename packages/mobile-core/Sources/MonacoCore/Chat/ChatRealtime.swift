import Foundation
import MonacoAPI

public enum ChatRealtimeEvent: Sendable, Equatable {
    case messageCreated(Components.Schemas.ChatMessage)
    case threadUpdated(id: String, replyCount: Int, lastReplyAt: Date)
    case seenUpdated(messageId: String, count: Int)
    case messageDeleted(id: String)
    case attached(resumed: Bool)
    case detached
}

public protocol ChatRealtime: Sendable {
    func events(cabalId: String) -> AsyncStream<ChatRealtimeEvent>
    func detach(cabalId: String)
}

public enum ChatEventDecoder {
    public static let messageCreated = "message.created"
    public static let threadUpdated = "thread.updated"
    public static let seenUpdated = "seen.updated"
    public static let messageDeleted = "message.deleted"

    public static func decode(name: String, data: Data) -> ChatRealtimeEvent? {
        switch name {
        case messageCreated:
            return decoded(Components.Schemas.ChatMessage.self, from: data).map(ChatRealtimeEvent.messageCreated)
        case threadUpdated:
            return decoded(ThreadUpdated.self, from: data).map {
                .threadUpdated(id: $0.messageId, replyCount: $0.replyCount, lastReplyAt: $0.lastReplyAt)
            }
        case seenUpdated:
            return decoded(SeenUpdated.self, from: data).map {
                .seenUpdated(messageId: $0.messageId, count: $0.count)
            }
        case messageDeleted:
            return decoded(MessageDeleted.self, from: data).map { .messageDeleted(id: $0.id) }
        default:
            return nil
        }
    }

    private static func decoded<Payload: Decodable>(_ type: Payload.Type, from data: Data) -> Payload? {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let text = try decoder.singleValueContainer().decode(String.self)
            guard let date = GroupChatDates.parse(text) else {
                throw DecodingError.dataCorrupted(
                    .init(codingPath: decoder.codingPath, debugDescription: "Expected an ISO8601 date.")
                )
            }
            return date
        }
        return try? decoder.decode(type, from: data)
    }
}

private struct ThreadUpdated: Decodable {
    let messageId: String
    let replyCount: Int
    let lastReplyAt: Date

    enum CodingKeys: String, CodingKey {
        case messageId = "message_id"
        case replyCount = "reply_count"
        case lastReplyAt = "last_reply_at"
    }
}

private struct SeenUpdated: Decodable {
    let messageId: String
    let count: Int

    enum CodingKeys: String, CodingKey {
        case messageId = "message_id"
        case count
    }
}

private struct MessageDeleted: Decodable {
    let id: String
}
