import Foundation
import MonacoAPI
import Observation

public struct CabalChatPreviewMessage: Equatable, Sendable {
    public let authorID: String
    public let author: String
    public let photoURL: String?
    public let body: String
    public let createdAt: Date

    public init(authorID: String, author: String, photoURL: String?, body: String, createdAt: Date) {
        self.authorID = authorID
        self.author = author
        self.photoURL = photoURL
        self.body = body
        self.createdAt = createdAt
    }

    public init?(_ message: Components.Schemas.ChatMessage) {
        guard !message.deleted, let body = message.body, !body.isEmpty else { return nil }
        let name = message.author.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        self.init(
            authorID: message.author.id,
            author: name.isEmpty ? message.author.handle.map { "@\($0)" } ?? "Member" : name,
            photoURL: message.author.photoUrl, body: body, createdAt: message.createdAt)
    }
}

@Observable
@MainActor
public final class CabalChatPreviewModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case empty
        case loaded(CabalChatPreviewMessage)
        case failed
    }

    public private(set) var phase: Phase = .loading
    public private(set) var unreadCount = 0
    public let cabalID: String

    private let api: APIClient
    private let hints: any HintSource
    private var generation = 0
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(cabalID: String, api: APIClient, hints: any HintSource) {
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
    }

    public func load() async {
        generation += 1
        let issued = generation
        let cabalID = cabalID
        do {
            async let messages = api.read { client in
                try await client.getChatMessages(path: .init(id: cabalID), query: .init(limit: 1)).ok.body.json
                    .messages
            }
            async let cabals = api.read { try await $0.getMyCabals().ok.body.json }
            let newest = try await messages.first.flatMap(CabalChatPreviewMessage.init)
            let unread = (try? await cabals)?.first { $0.id == cabalID }?.unreadCount ?? 0
            guard issued == generation else { return }
            unreadCount = unread
            phase = newest.map(Phase.loaded) ?? .empty
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            if case .loaded = phase { return }
            phase = .failed
        }
    }

    public func observe() async {
        await refresher.observe([
            hints.hints(matching: .cabal(id: cabalID, what: nil)),
            hints.hints(matching: .user(what: "cabals")),
        ])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
