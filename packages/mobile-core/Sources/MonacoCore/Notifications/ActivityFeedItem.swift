import Foundation

public struct ActivityFeedBadge: Equatable, Sendable {
    public enum Tone: Equatable, Sendable {
        case warn
        case positive
        case neutral
    }

    public let text: String
    public let tone: Tone

    public init(_ text: String, tone: Tone) {
        self.text = text
        self.tone = tone
    }
}

public struct ActivityFeedItem: Identifiable, Equatable, Sendable {
    public enum Kind: Equatable, Sendable {
        case reply
        case vote
        case proposalPassed
        case needsVote
        case accountUpdate
    }

    public let id: String
    public let kind: Kind
    public let title: String
    public let body: String
    public let cabalName: String?
    public let age: String
    public let actorName: String?
    public let actorID: String?
    public let actorPhotoURL: String?
    public let badge: ActivityFeedBadge?
    public let destination: PushRoute
    public internal(set) var isRead: Bool

    public init(
        id: String, kind: Kind, title: String, body: String, cabalName: String? = nil, age: String,
        actorName: String? = nil, actorID: String? = nil, actorPhotoURL: String? = nil, badge: ActivityFeedBadge? = nil,
        destination: PushRoute, isRead: Bool = false
    ) {
        self.id = id
        self.kind = kind
        self.title = title
        self.body = body
        self.cabalName = cabalName
        self.age = age
        self.actorName = actorName
        self.actorID = actorID
        self.actorPhotoURL = actorPhotoURL
        self.badge = badge
        self.destination = destination
        self.isRead = isRead
    }

    public var metaLine: String {
        switch kind {
        case .accountUpdate: [body, age].joined(separator: " · ")
        case .reply, .vote, .proposalPassed, .needsVote: [cabalName, age].compactMap { $0 }.joined(separator: " · ")
        }
    }

    public var spokenText: String {
        let parts: [String]
        switch kind {
        case .accountUpdate: parts = [title, metaLine]
        case .reply, .vote, .proposalPassed, .needsVote: parts = [title, body, metaLine, badge?.text].compactMap { $0 }
        }
        return ((isRead ? [] : ["Unread"]) + parts).joined(separator: ", ")
    }
}
