import Foundation
import MonacoAPI
import MonacoFlows
import Observation

public struct CatchUpScreen: Equatable, Sendable {
    public typealias Item = Components.Schemas.CatchUpItem

    public private(set) var count: Int
    public let cabals: [Components.Schemas.CatchUpCabal]
    public private(set) var waiting: [Item]
    public private(set) var replies: [Item]
    public private(set) var invites: [Item]
    public private(set) var results: [Item]
    public private(set) var cleared: [Item]

    public var isCaughtUp: Bool { count == 0 }

    public init(_ read: Components.Schemas.CatchUp) {
        count = read.count
        cabals = read.cabals
        waiting = read.waiting
        replies = read.replies
        invites = read.invites
        results = read.results
        cleared = read.cleared
    }

    public func item(key: String) -> Item? {
        (waiting + replies + invites + results).first { $0.key == key }
    }

    fileprivate enum Section { case waiting, replies, invites, results }

    fileprivate struct Slot {
        let section: Section
        let index: Int
    }

    private func slot(of key: String) -> Slot? {
        for (section, items) in [
            (Section.waiting, waiting), (.replies, replies), (.invites, invites), (.results, results),
        ] {
            if let index = items.firstIndex(where: { $0.key == key }) { return Slot(section: section, index: index) }
        }
        return nil
    }

    private mutating func take(_ slot: Slot) -> Item {
        switch slot.section {
        case .waiting: waiting.remove(at: slot.index)
        case .replies: replies.remove(at: slot.index)
        case .invites: invites.remove(at: slot.index)
        case .results: results.remove(at: slot.index)
        }
    }

    private mutating func put(_ item: Item, at slot: Slot) {
        switch slot.section {
        case .waiting: waiting.insert(item, at: min(slot.index, waiting.count))
        case .replies: replies.insert(item, at: min(slot.index, replies.count))
        case .invites: invites.insert(item, at: min(slot.index, invites.count))
        case .results: results.insert(item, at: min(slot.index, results.count))
        }
    }

    fileprivate mutating func clear(_ key: String) -> (item: Item, slot: Slot)? {
        guard let slot = slot(of: key) else { return nil }
        let item = take(slot)
        var moved = item
        moved.clearedReason = .cleared
        moved.unread = false
        cleared.insert(moved, at: 0)
        count = max(0, count - 1)
        return (item, slot)
    }

    fileprivate mutating func restore(_ item: Item, at slot: Slot) {
        cleared.removeAll { $0.key == item.key }
        put(item, at: slot)
        count += 1
    }

    fileprivate mutating func setUnread(_ unread: Bool, key: String) {
        guard let slot = slot(of: key) else { return }
        switch slot.section {
        case .waiting: waiting[slot.index].unread = unread
        case .replies: replies[slot.index].unread = unread
        case .invites: invites[slot.index].unread = unread
        case .results: results[slot.index].unread = unread
        }
    }
}

@Observable
@MainActor
public final class CatchUpModel {
    public private(set) var state: LoadState<CatchUpScreen> = .idle

    private let api: APIClient
    private let hints: any HintSource
    private let clock: @Sendable () -> Date
    private let calendar: Calendar
    private var generation = 0
    private var writesInFlight = 0
    private var reloadAfterWrites = false
    private let clearSubmission = IdempotentSubmission()
    private let keepSubmission = IdempotentSubmission()
    private let readSubmission = IdempotentSubmission()
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(
        api: APIClient, hints: any HintSource, clock: @escaping @Sendable () -> Date, calendar: Calendar = .current
    ) {
        self.api = api
        self.hints = hints
        self.clock = clock
        self.calendar = calendar
    }

    public func load() async {
        guard writesInFlight == 0 else {
            reloadAfterWrites = true
            return
        }
        generation += 1
        let mine = generation
        let since = calendar.startOfDay(for: clock())
        if !isLoaded { state = .loading }
        do {
            let read = try await api.read { client in
                try await client.getMyCatchUp(query: .init(since: since)).ok.body.json
            }
            guard mine == generation else { return }
            state = .loaded(CatchUpScreen(read))
        } catch {
            guard mine == generation, !isLoaded else { return }
            state = .failed(APIError(error))
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .user(what: "catch_up_changed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    @discardableResult
    public func clear(_ key: String) async -> Bool {
        guard case .loaded(var screen) = state, let removed = screen.clear(key) else { return false }
        state = .loaded(screen)
        let ok = await write(
            submission: clearSubmission, key: key, operation: Operations.ClearCatchUpItem.id
        ) { client, key, idempotencyKey in
            _ = try await client.clearCatchUpItem(
                path: .init(key: key), headers: .init(idempotencyKey: idempotencyKey)
            ).noContent
        }
        if !ok, case .loaded(var current) = state {
            current.restore(removed.item, at: removed.slot)
            state = .loaded(current)
        }
        await settle()
        return ok
    }

    public func keep(_ key: String) async {
        await markRead(key, submission: keepSubmission, operation: Operations.KeepCatchUpItem.id) {
            client, key, idempotencyKey in
            _ = try await client.keepCatchUpItem(
                path: .init(key: key), headers: .init(idempotencyKey: idempotencyKey)
            ).noContent
        }
    }

    public func read(_ key: String) async {
        await markRead(key, submission: readSubmission, operation: Operations.ReadCatchUpItem.id) {
            client, key, idempotencyKey in
            _ = try await client.readCatchUpItem(
                path: .init(key: key), headers: .init(idempotencyKey: idempotencyKey)
            ).noContent
        }
    }

    private func markRead(
        _ key: String, submission: IdempotentSubmission, operation: String,
        call: @escaping @Sendable (Client, String, String) async throws -> Void
    ) async {
        guard case .loaded(var screen) = state, screen.item(key: key)?.unread == true else { return }
        screen.setUnread(false, key: key)
        state = .loaded(screen)
        let ok = await write(submission: submission, key: key, operation: operation, call: call)
        if !ok, case .loaded(var current) = state {
            current.setUnread(true, key: key)
            state = .loaded(current)
        }
        await settle()
    }

    private func write(
        submission: IdempotentSubmission, key: String, operation: String,
        call: @escaping @Sendable (Client, String, String) async throws -> Void
    ) async -> Bool {
        writesInFlight += 1
        defer { writesInFlight -= 1 }
        do {
            try await api.submit(submission, payload: key, operation: operation) { client, idempotencyKey in
                try await call(client, key, idempotencyKey)
            }
            return true
        } catch {
            return false
        }
    }

    private func settle() async {
        guard writesInFlight == 0, reloadAfterWrites else { return }
        reloadAfterWrites = false
        await load()
    }

    private var isLoaded: Bool {
        if case .loaded = state { return true }
        return false
    }
}

#if DEBUG
extension CatchUpModel {
    public static func preview(state: LoadState<CatchUpScreen>, now: Date) -> CatchUpModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        let model = CatchUpModel(
            api: APIClient(serverURL: serverURL, tokens: PreviewTokens(), transport: PreviewTransport()),
            hints: PreviewHints(), clock: { now })
        model.state = state
        return model
    }
}

private struct PreviewTransport: ClientTransport {
    func send(
        _: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        (HTTPResponse(status: .noContent), nil)
    }
}

private struct PreviewTokens: AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct PreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
