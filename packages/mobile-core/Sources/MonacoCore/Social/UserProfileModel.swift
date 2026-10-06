import Foundation
import MonacoAPI
import Observation

public struct FollowListUser: Identifiable, Equatable, Sendable {
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

    init(_ user: Components.Schemas.FollowUser) {
        self.init(
            id: user.userId, handle: user.handle, displayName: user.displayName, photoURL: user.photoUrl,
            followedByMe: user.followedByMe)
    }
}

@Observable
@MainActor
public final class UserProfileModel {
    public enum Phase: Equatable, Sendable {
        case idle
        case loading
        case loaded
        case unavailable
        case failed
    }

    public enum ListPhase: Equatable, Sendable {
        case loading
        case empty
        case failed
        case loaded
    }

    public let userID: String
    public private(set) var phase: Phase = .idle
    public private(set) var profile: Components.Schemas.PublicProfile?
    public private(set) var followedByMe = false
    public private(set) var lastError: APIError?
    public private(set) var toastTick = 0

    private let api: APIClient
    private var loadGeneration = 0
    private var toggling: Set<String> = []
    private var overrides: [String: Bool] = [:]

    @ObservationIgnored private lazy var followersPager: CursorPager<FollowListUser> = makePager(.followers)
    @ObservationIgnored private lazy var followingPager: CursorPager<FollowListUser> = makePager(.following)

    public init(userID: String, api: APIClient) {
        self.userID = userID
        self.api = api
    }

    public var displayName: String { profile?.displayName ?? "" }

    public var handle: String { profile?.handle ?? "" }

    public var photoURL: String? { profile?.photoUrl }

    public var followerCount: Int { profile?.followerCount ?? 0 }

    public var followingCount: Int { profile?.followingCount ?? 0 }

    public var followerRows: [FollowListUser] { applying(followersPager.items) }

    public var followingRows: [FollowListUser] { applying(followingPager.items) }

    public var followersPhase: ListPhase { listPhase(followersPager) }

    public var followingPhase: ListPhase { listPhase(followingPager) }

    public var followersLoadingMore: Bool { followersPager.phase == .loadingMore }

    public var followingLoadingMore: Bool { followingPager.phase == .loadingMore }

    public var followersPageFailed: Bool { pageFailed(followersPager) }

    public var followingPageFailed: Bool { pageFailed(followingPager) }

    public func isToggling(_ userID: String) -> Bool {
        toggling.contains(userID)
    }

    public func load() async {
        await reload(keepingVisible: false)
    }

    public func refresh() async {
        await reload(keepingVisible: profile != nil)
    }

    public func follow(userID: String, source: String = "profile") async {
        await change(userID, to: true, source: source)
    }

    public func unfollow(userID: String) async {
        await change(userID, to: false, source: "profile")
    }

    public func loadFollowers() async {
        await followersPager.loadFirst()
    }

    public func loadMoreFollowers() async {
        await followersPager.loadMore()
    }

    public func loadFollowing() async {
        await followingPager.loadFirst()
    }

    public func loadMoreFollowing() async {
        await followingPager.loadMore()
    }

    private func reload(keepingVisible: Bool) async {
        loadGeneration += 1
        let mine = loadGeneration
        if !keepingVisible { phase = .loading }
        do {
            let next = try await Self.readProfile(userID: userID, api: api)
            guard mine == loadGeneration else { return }
            profile = next
            overrides.removeValue(forKey: userID)
            followedByMe = next.followedByMe
            phase = .loaded
        } catch {
            guard mine == loadGeneration else { return }
            let failure = APIError(error)
            if Self.meansUnavailable(failure) {
                markUnavailable()
                return
            }
            if keepingVisible, profile != nil {
                note(failure)
            } else {
                phase = .failed
            }
        }
    }

    private func change(_ target: String, to following: Bool, source: String) async {
        guard !toggling.contains(target) else { return }
        let previous = displayedFollowing(target)
        guard previous != following else { return }
        loadGeneration += 1
        toggling.insert(target)
        defer { toggling.remove(target) }
        apply(target, following: following)
        do {
            let state = try await request(following: following, userID: target, source: source)
            settle(target, following: state.following)
            if target == userID { await reload(keepingVisible: true) }
        } catch {
            settle(target, following: previous)
            let failure = APIError(error)
            if target == userID, Self.meansUnavailable(failure) {
                markUnavailable()
            } else {
                note(failure)
            }
        }
    }

    private func request(
        following: Bool, userID target: String, source: String
    ) async throws -> Components.Schemas.FollowState {
        let submission = IdempotentSubmission()
        let call = FollowCall(userID: target, source: source, following: following)
        if following {
            return try await api.submit(submission, payload: call, operation: "postUserFollow") { client, key in
                try await client.postUserFollow(
                    path: .init(id: target),
                    headers: .init(idempotencyKey: key),
                    body: .json(.init(source: source))
                ).ok.body.json
            }
        }
        return try await api.submit(submission, payload: call, operation: "deleteUserFollow") { client, key in
            try await client.deleteUserFollow(
                path: .init(id: target),
                headers: .init(idempotencyKey: key)
            ).ok.body.json
        }
    }

    private func apply(_ target: String, following: Bool) {
        overrides[target] = following
        if target == userID { followedByMe = following }
    }

    private func settle(_ target: String, following: Bool) {
        overrides.removeValue(forKey: target)
        followersPager.update(id: target) { $0.followedByMe = following }
        followingPager.update(id: target) { $0.followedByMe = following }
        if target == userID { followedByMe = following }
    }

    private func acceptPage(_ items: [FollowListUser]) {
        for item in items where !toggling.contains(item.id) {
            overrides.removeValue(forKey: item.id)
        }
    }

    private func displayedFollowing(_ target: String) -> Bool {
        if let override = overrides[target] { return override }
        if let row = followersPager.items.first(where: { $0.id == target }) { return row.followedByMe }
        if let row = followingPager.items.first(where: { $0.id == target }) { return row.followedByMe }
        return target == userID ? followedByMe : false
    }

    private func applying(_ items: [FollowListUser]) -> [FollowListUser] {
        items.map { item in
            guard let followed = overrides[item.id] else { return item }
            var copy = item
            copy.followedByMe = followed
            return copy
        }
    }

    private func pageFailed(_ pager: CursorPager<FollowListUser>) -> Bool {
        if case .failed = pager.phase { return !pager.items.isEmpty }
        return false
    }

    private func listPhase(_ pager: CursorPager<FollowListUser>) -> ListPhase {
        if !pager.items.isEmpty { return .loaded }
        return switch pager.phase {
        case .failed: .failed
        case .exhausted: .empty
        case .idle, .loadingFirst, .loadingMore: .loading
        }
    }

    private func markUnavailable() {
        profile = nil
        followedByMe = false
        overrides.removeValue(forKey: userID)
        phase = .unavailable
    }

    private func note(_ error: APIError) {
        lastError = error
        toastTick += 1
    }

    private enum Directory {
        case followers
        case following
    }

    private func makePager(_ directory: Directory) -> CursorPager<FollowListUser> {
        let api = api
        let userID = userID
        return CursorPager { [weak self] cursor in
            do {
                let page = try await Self.directoryPage(directory, userID: userID, cursor: cursor, api: api)
                await self?.acceptPage(page.items)
                return page
            } catch {
                let failure = APIError(error)
                let visible = await self?.directoryHasRows(directory) ?? false
                if cursor != nil || visible { await self?.note(failure) }
                throw error
            }
        }
    }

    private func directoryHasRows(_ directory: Directory) -> Bool {
        switch directory {
        case .followers: !followersPager.items.isEmpty
        case .following: !followingPager.items.isEmpty
        }
    }

    private static func readProfile(userID: String, api: APIClient) async throws -> Components.Schemas.PublicProfile {
        try await api.read { client in
            try await client.getUser(path: .init(id: userID)).ok.body.json
        }
    }

    private static func directoryPage(
        _ directory: Directory, userID: String, cursor: String?, api: APIClient
    ) async throws -> (items: [FollowListUser], nextCursor: String?) {
        let page = try await api.read { client in
            switch directory {
            case .followers:
                try await client.getUserFollowers(
                    path: .init(id: userID), query: .init(cursor: cursor, limit: 30)
                ).ok.body.json
            case .following:
                try await client.getUserFollowing(
                    path: .init(id: userID), query: .init(cursor: cursor, limit: 30)
                ).ok.body.json
            }
        }
        return (page.items.map(FollowListUser.init), page.nextCursor)
    }

    private struct FollowCall: Encodable, Sendable {
        var userID: String
        var source: String
        var following: Bool
    }

    private static func meansUnavailable(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error else { return false }
        return problem.code.wire == "user_not_found" || problem.code.wire == "user_banned"
    }
}
