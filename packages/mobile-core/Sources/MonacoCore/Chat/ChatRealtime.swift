import Foundation
import MonacoAPI

public struct ChatReactionCount: Sendable, Equatable, Decodable {
    public let emoji: String
    public let count: Int32

    public init(emoji: String, count: Int32) {
        self.emoji = emoji
        self.count = count
    }
}

public enum ChatRealtimeEvent: Sendable, Equatable {
    case messageCreated(Components.Schemas.ChatMessage)
    case momentCreated(Components.Schemas.ChatMessage)
    case threadUpdated(id: String, replyCount: Int, lastReplyAt: Date)
    case seenUpdated(messageId: String, count: Int)
    case messageDeleted(id: String)
    case reactionUpdated(messageId: String, reactions: [ChatReactionCount], total: Int32)
    case pollUpdated(messageId: String, options: [ChatPollCount], total: Int)
    case attached(resumed: Bool)
    case detached
}

public protocol ChatRealtime: Sendable {
    func events(cabalId: String) -> AsyncStream<ChatRealtimeEvent>
    func detach(cabalId: String)
}

public enum ChatEventDecoder {
    public static let messageCreated = "message.created"
    public static let momentCreated = "moment.created"
    public static let threadUpdated = "thread.updated"
    public static let seenUpdated = "seen.updated"
    public static let messageDeleted = "message.deleted"
    public static let reactionUpdated = "reaction.updated"
    public static let pollUpdated = "poll.updated"

    public static func decode(name: String, data: Data) -> ChatRealtimeEvent? {
        switch name {
        case messageCreated:
            let message =
                decoded(Components.Schemas.ChatMessage.self, from: data)
                ?? withReactionDefaults(data).flatMap { decoded(Components.Schemas.ChatMessage.self, from: $0) }
            return message.map(ChatRealtimeEvent.messageCreated)
        case momentCreated:
            return decoded(Components.Schemas.ChatMessage.self, from: data).map(ChatRealtimeEvent.momentCreated)
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
        case reactionUpdated:
            return decoded(ReactionUpdated.self, from: data).map {
                .reactionUpdated(messageId: $0.messageId, reactions: $0.reactions, total: $0.total)
            }
        case pollUpdated:
            return decoded(PollUpdated.self, from: data).map {
                .pollUpdated(messageId: $0.messageId, options: $0.options, total: $0.totalVotes)
            }
        default:
            return nil
        }
    }

    private static func withReactionDefaults(_ data: Data) -> Data? {
        guard var object = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] else { return nil }
        object["reactions"] = object["reactions"] ?? []
        object["reaction_count"] = object["reaction_count"] ?? 0
        return try? JSONSerialization.data(withJSONObject: object)
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

private struct ReactionUpdated: Decodable {
    let messageId: String
    let reactions: [ChatReactionCount]
    let total: Int32

    enum CodingKeys: String, CodingKey {
        case messageId = "message_id"
        case reactions
        case total
    }
}

private struct PollUpdated: Decodable {
    let messageId: String
    let options: [ChatPollCount]
    let totalVotes: Int

    enum CodingKeys: String, CodingKey {
        case messageId = "message_id"
        case options
        case totalVotes = "total_votes"
    }
}
