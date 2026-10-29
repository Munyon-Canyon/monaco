import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class InviteLandingModel {
    public enum Redirect: Equatable, Sendable {
        case cabal(String)
        case referrer(String)
        case signIn
    }

    public enum Phase: Equatable, Sendable {
        case loading
        case loaded
        case failed
    }

    public static let refreshInterval: Duration = .seconds(60)

    public let code: String
    public let cabalID: String
    public private(set) var phase: Phase = .loading
    public private(set) var preview: Components.Schemas.ReferralCabal?
    public private(set) var redirect: Redirect?
    public var toast: String?

    private let api: APIClient
    private let hints: (any HintSource)?
    private let viewerID: String?
    private let pendingStore: PendingLinkTargetStore
    private let now: () -> Date
    private let sleep: @Sendable (Duration) async throws -> Void
    private let access: CabalAccessModel?
    private var accessLoaded = false
    private var autoJoinChecked = false
    private var joining = false

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(
        code: String, cabalID: String, api: APIClient, hints: (any HintSource)?, viewerID: String?,
        pendingStore: PendingLinkTargetStore = .init(), now: @escaping () -> Date = Date.init,
        sleep: @escaping @Sendable (Duration) async throws -> Void = { try await Task.sleep(for: $0) }
    ) {
        self.code = code
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
        self.viewerID = viewerID
        self.pendingStore = pendingStore
        self.now = now
        self.sleep = sleep
        access = viewerID == nil ? nil : CabalAccessModel(cabalID: cabalID, api: api, hints: hints ?? NoHints())
        access?.viaCode = code
    }

    public var content: InviteLandingContent? {
        preview.flatMap { InviteLandingCopy.content($0, now: now()) }
    }

    private var name: String { content?.name ?? "" }

    private var standing: CabalAccessStanding? { accessLoaded ? access?.standing : nil }

    private var policy: CabalJoinPolicy { access?.joinPolicy ?? content?.joinMode ?? .request }

    public var buttonTitle: String {
        if case .requested = standing { return InviteLandingCopy.requestSentTitle }
        return InviteLandingCopy.buttonTitle(name: name, policy: policy)
    }

    public var buttonEnabled: Bool {
        guard phase == .loaded, content != nil, !joining else { return false }
        guard access != nil else { return true }
        switch standing {
        case .join, .declined: return true
        default: return false
        }
    }

    public var showsAcceptDecline: Bool {
        if case .invited = standing { return true }
        return false
    }

    public var note: String? {
        viewerID == nil ? InviteLandingCopy.signedOutNote : nil
    }

    public func load() async {
        let loaded: Components.Schemas.ReferralCabal
        do {
            let code = code
            let cabalID = cabalID
            loaded = try await api.read { client in
                try await client.getReferralCabal(path: .init(code: code, cabalId: cabalID)).ok.body.json
            }
        } catch {
            if preview == nil { phase = .failed }
            return
        }
        preview = loaded
        phase = .loaded
        guard loaded.cabal != nil else {
            redirect = .referrer(code)
            return
        }
        guard let access else { return }
        let cabalID = cabalID
        let cabal: Components.Schemas.Cabal
        do {
            cabal = try await api.read { client in
                try await client.getCabal(path: .init(id: cabalID)).ok.body.json
            }
        } catch {
            toast = ToastCopy.message(for: APIError(error))
            return
        }
        if cabal.me != nil {
            _ = pendingStore.takeJoin(cabalID: cabalID, now: now())
            redirect = .cabal(cabalID)
            return
        }
        await access.load(cabal: cabal)
        accessLoaded = true
        guard !autoJoinChecked else { return }
        autoJoinChecked = true
        if pendingStore.takeJoin(cabalID: cabalID, now: now()), buttonEnabled { await join() }
    }

    public func join() async {
        guard phase == .loaded, let content, !joining else { return }
        guard let access else {
            let target = PendingLinkTarget(
                kind: .join, code: code, cabalID: cabalID, proposalID: nil, joinTapped: true, savedAt: now())
            pendingStore.save(target)
            redirect = .signIn
            return
        }
        guard buttonEnabled else { return }
        joining = true
        defer { joining = false }
        let serial = access.toast?.serial ?? 0
        let changes = access.membershipChanges
        await access.enter()
        if access.membershipChanges > changes {
            redirect = .cabal(cabalID)
        } else if case .requested = access.standing {
            let sent = access.toast.map { $0.serial > serial && $0.message == CabalEntry.requestedToast } ?? false
            toast = sent ? InviteLandingCopy.requestSentToast(name: content.name) : InviteLandingCopy.pendingToast
        } else if let shown = access.toast, shown.serial > serial {
            toast = isBanned(access.lastRefusal) ? InviteLandingCopy.bannedToast : shown.message
        }
    }

    public func accept() async {
        await answer { await $0.accept() }
    }

    public func decline() async {
        await answer { await $0.decline() }
    }

    private func answer(_ action: (CabalAccessModel) async -> Void) async {
        guard let access, showsAcceptDecline else { return }
        let serial = access.toast?.serial ?? 0
        let changes = access.membershipChanges
        await action(access)
        if access.membershipChanges > changes {
            redirect = .cabal(cabalID)
        } else if let shown = access.toast, shown.serial > serial {
            toast = shown.message
        }
    }

    public func run() async {
        if phase == .loading { await load() }
        guard viewerID != nil else {
            while !Task.isCancelled {
                do { try await sleep(Self.refreshInterval) } catch { return }
                await load()
            }
            return
        }
        guard let hints else { return }
        await refresher.observe([hints.hints(matching: .cabal(id: cabalID, what: "proposal_updated"))])
    }

    private func isBanned(_ error: APIError?) -> Bool {
        guard case .problem(let problem) = error else { return false }
        return problem.code.wire == "cabal_banned"
    }
}

private struct NoHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
