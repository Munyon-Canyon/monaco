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

    enum Section: CaseIterable {
        case waiting, replies, invites, results
    }

    private subscript(section: Section) -> [Item] {
        get {
            switch section {
            case .waiting: waiting
            case .replies: replies
            case .invites: invites
            case .results: results
            }
        }
        set {
            switch section {
            case .waiting: waiting = newValue
            case .replies: replies = newValue
            case .invites: invites = newValue
            case .results: results = newValue
            }
        }
    }

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

    struct Slot {
        let section: Section
        let index: Int
        let item: Item
    }

    func slot(of key: String) -> Slot? {
        for section in Section.allCases {
            if let index = self[section].firstIndex(where: { $0.key == key }) {
                return Slot(section: section, index: index, item: self[section][index])
            }
        }
        return nil
    }

    func clearing(_ slot: Slot) -> CatchUpScreen {
        var next = self
        var item = next[slot.section].remove(at: slot.index)
        item.clearedReason = .cleared
        next.cleared.insert(item, at: 0)
        next.count = max(0, count - 1)
        return next
    }

    func reinstating(_ slot: Slot) -> CatchUpScreen {
        var next = self
        next.cleared.removeAll { $0.key == slot.item.key }
        let index = min(slot.index, next[slot.section].count)
        next[slot.section].insert(slot.item, at: index)
        next.count = count + 1
        return next
    }

    func settingUnread(_ unread: Bool, forKey key: String) -> CatchUpScreen {
        var next = self
        for section in Section.allCases {
            if let index = next[section].firstIndex(where: { $0.key == key }) {
                next[section][index].unread = unread
            }
        }
        return next
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
    private let clearSubmission = IdempotentSubmission()
    private let keepSubmission = IdempotentSubmission()
    private let readSubmission = IdempotentSubmission()
    private var generation = 0
    private var writesInFlight = 0
    private var reloadAfterWrites = false
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
        if writesInFlight > 0 {
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

    public func clear(_ key: String) async -> Bool {
        guard case .loaded(let screen) = state, let slot = screen.slot(of: key) else { return false }
        state = .loaded(screen.clearing(slot))
        writesInFlight += 1
        var succeeded = true
        do {
            _ = try await api.submit(clearSubmission, payload: key, operation: "clearCatchUpItem") { client, idem in
                try await client.clearCatchUpItem(path: .init(key: key), headers: .init(idempotencyKey: idem))
                    .noContent
            }
        } catch {
            succeeded = false
            if case .loaded(let current) = state { state = .loaded(current.reinstating(slot)) }
        }
        await writeSettled()
        return succeeded
    }

    public func keep(_ key: String) async -> Bool {
        guard let wasUnread = beginMarkingRead(key) else { return false }
        var succeeded = true
        do {
            _ = try await api.submit(keepSubmission, payload: key, operation: "keepCatchUpItem") { client, idem in
                try await client.keepCatchUpItem(path: .init(key: key), headers: .init(idempotencyKey: idem))
                    .noContent
            }
        } catch {
            succeeded = false
            restoreUnread(key, wasUnread)
        }
        await writeSettled()
        return succeeded
    }

    public func read(_ key: String) async -> Bool {
        guard let wasUnread = beginMarkingRead(key) else { return false }
        var succeeded = true
        do {
            _ = try await api.submit(readSubmission, payload: key, operation: "readCatchUpItem") { client, idem in
                try await client.readCatchUpItem(path: .init(key: key), headers: .init(idempotencyKey: idem))
                    .noContent
            }
        } catch {
            succeeded = false
            restoreUnread(key, wasUnread)
        }
        await writeSettled()
        return succeeded
    }

    private func beginMarkingRead(_ key: String) -> Bool? {
        guard case .loaded(let screen) = state, let slot = screen.slot(of: key) else { return nil }
        state = .loaded(screen.settingUnread(false, forKey: key))
        writesInFlight += 1
        return slot.item.unread
    }

    private func restoreUnread(_ key: String, _ wasUnread: Bool) {
        guard wasUnread, case .loaded(let current) = state else { return }
        state = .loaded(current.settingUnread(true, forKey: key))
    }

    private func writeSettled() async {
        writesInFlight -= 1
        guard writesInFlight == 0, reloadAfterWrites else { return }
        reloadAfterWrites = false
        await load()
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .user(what: "catch_up_changed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
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
        _ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        guard request.method == .post else { throw URLError(.notConnectedToInternet) }
        return (HTTPResponse(status: .noContent), nil)
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
