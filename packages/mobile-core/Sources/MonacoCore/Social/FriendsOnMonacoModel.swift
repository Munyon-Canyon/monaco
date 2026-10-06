import Foundation
import MonacoAPI
import Observation

public struct FriendOnMonaco: Identifiable, Equatable, Sendable {
    public let id: String
    public let handle: String
    public let displayName: String
    public let photoURL: String?
    public var followedByMe: Bool

    public init(id: String, handle: String, displayName: String, photoURL: String?, followedByMe: Bool) {
        self.id = id
        self.handle = handle
        self.displayName = displayName
        self.photoURL = photoURL
        self.followedByMe = followedByMe
    }

    init(_ match: Components.Schemas.ContactMatch) {
        self.init(
            id: match.userId, handle: match.handle, displayName: match.displayName, photoURL: match.photoUrl,
            followedByMe: match.followedByMe)
    }
}

@Observable
@MainActor
public final class FriendsOnMonacoModel {
    public enum Phase: Equatable, Sendable {
        case idle
        case checking
        case loaded
        case empty
        case failed
    }

    public private(set) var access: ContactsAccess
    public private(set) var phase: Phase = .idle
    public private(set) var friends: [FriendOnMonaco] = []
    public private(set) var toast: String?
    public private(set) var toastTick = 0

    private let api: APIClient
    private let contacts: any ContactsSource
    private let defaultRegion: String
    private let chunkSize: Int
    private var generation = 0
    private var toggling: Set<String> = []

    public init(
        api: APIClient, contacts: any ContactsSource, defaultRegion: String,
        chunkSize: Int = ContactHashing.chunkSize
    ) {
        self.api = api
        self.contacts = contacts
        self.defaultRegion = defaultRegion
        self.chunkSize = chunkSize
        access = contacts.currentAccess()
    }

    public func isToggling(_ friendID: String) -> Bool {
        toggling.contains(friendID)
    }

    public func findFriends() async {
        access = await contacts.requestAccess()
        guard access == .granted else { return }
        phase = .checking
        await upload()
    }

    public func loadIfGranted() async {
        access = contacts.currentAccess()
        guard access == .granted, phase == .idle else { return }
        await upload()
    }

    public func refreshAndLoad() async {
        let previous = access
        access = contacts.currentAccess()
        guard access == .granted, phase != .checking else { return }
        guard previous != .granted || phase == .idle || phase == .failed else { return }
        await upload()
    }

    public func retry() async {
        guard phase != .checking else { return }
        await upload()
    }

    public func follow(_ friendID: String) async {
        guard let index = friends.firstIndex(where: { $0.id == friendID }) else { return }
        guard !friends[index].followedByMe, !toggling.contains(friendID) else { return }
        toggling.insert(friendID)
        defer { toggling.remove(friendID) }
        friends[index].followedByMe = true
        do {
            let following = try await sendFollow(friendID)
            if let current = friends.firstIndex(where: { $0.id == friendID }) {
                friends[current].followedByMe = following
            }
        } catch {
            if let current = friends.firstIndex(where: { $0.id == friendID }) {
                friends[current].followedByMe = false
            }
            show(APIError(error))
        }
    }

    private func upload() async {
        generation += 1
        let mine = generation
        phase = .checking
        friends = []
        toast = nil
        do {
            let numbers = try contacts.phoneNumbers()
            let digests = ContactHashing.hashes(for: numbers, defaultRegion: defaultRegion)
            try await post(ContactHashing.chunks(digests, size: chunkSize))
            let loaded = try await loadPages()
            guard mine == generation else { return }
            friends = loaded
            phase = loaded.isEmpty ? .empty : .loaded
        } catch {
            guard mine == generation else { return }
            let failure = APIError(error)
            if failure.status == 429 { show(failure) }
            phase = .failed
        }
    }

    private func post(_ chunks: [[String]]) async throws {
        for chunk in chunks {
            let submission = IdempotentSubmission()
            _ = try await api.submit(submission, payload: chunk, operation: "postMeContactsMatch") { client, key in
                try await client.postMeContactsMatch(
                    headers: .init(idempotencyKey: key),
                    body: .json(.init(hashes: chunk))
                ).ok.body.json
            }
        }
    }

    private func loadPages() async throws -> [FriendOnMonaco] {
        var cursor: String?
        var rows: [FriendOnMonaco] = []
        var seen: Set<String> = []
        for _ in 0..<40 {
            let pageCursor = cursor
            let page = try await api.read { client in
                try await client.getMeContactsMatches(query: .init(cursor: pageCursor, limit: 50)).ok.body.json
            }
            for item in page.items where seen.insert(item.userId).inserted {
                rows.append(FriendOnMonaco(item))
            }
            guard let next = page.nextCursor, !next.isEmpty, next != cursor else { return rows }
            cursor = next
        }
        return rows
    }

    private func sendFollow(_ friendID: String) async throws -> Bool {
        let submission = IdempotentSubmission()
        let state = try await api.submit(submission, payload: friendID, operation: "postUserFollow") { client, key in
            try await client.postUserFollow(
                path: .init(id: friendID),
                headers: .init(idempotencyKey: key),
                body: .json(.init(source: "phone"))
            ).ok.body.json
        }
        return state.following
    }

    private func show(_ error: APIError) {
        toast = ToastCopy.message(for: error)
        toastTick += 1
    }
}

extension APIError {
    fileprivate var status: Int? {
        guard case .problem(let problem) = self else { return nil }
        return problem.status
    }
}
