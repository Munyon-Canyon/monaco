import Foundation
import MonacoAPI
import Observation

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public struct InviteLinks: Equatable, Sendable {
    public let shareURL: URL
    public let codeURL: URL?
    public let showsUnlockPrompt: Bool

    public init?(_ code: Components.Schemas.MyReferralCode) {
        guard let link = URL(string: code.link) else { return nil }
        let handleLink = code.handleUnlocked ? code.handleLink.flatMap(URL.init(string:)) : nil
        shareURL = handleLink ?? link
        codeURL = handleLink == nil ? nil : link
        showsUnlockPrompt = !code.handleUnlocked
    }

    public static func withoutScheme(_ url: URL) -> String {
        "\(url.host ?? "")\(url.path)"
    }

    public static func unlockCopy(handle: String?) -> String {
        let name = handle.map { "@\($0)" } ?? "your handle"
        return "Make your first deposit to use \(name) as your invite link"
    }
}

public enum InviteLinkState: Equatable, Sendable {
    case idle
    case loading
    case pending
    case loaded(InviteLinks)
    case failed(APIError)
}

@Observable
@MainActor
public final class InviteLinkModel {
    public static let pendingRetryDelay: Duration = .seconds(2)

    public private(set) var state: InviteLinkState = .idle
    public private(set) var toast: String?

    private let api: APIClient
    private let hints: any HintSource
    private let sleep: @Sendable (Duration) async throws -> Void
    private var generation = 0

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(
        api: APIClient,
        hints: any HintSource,
        sleep: @escaping @Sendable (Duration) async throws -> Void = { try await Task.sleep(for: $0) }
    ) {
        self.api = api
        self.hints = hints
        self.sleep = sleep
    }

    public var links: InviteLinks? {
        if case .loaded(let links) = state { return links }
        return nil
    }

    public func load() async {
        generation += 1
        let current = generation
        if links == nil { state = .loading }
        var result = await fetch()
        if case .failure(let error) = result, Self.isPending(error) {
            guard current == generation else { return }
            if links == nil { state = .pending }
            do { try await sleep(Self.pendingRetryDelay) } catch { return }
            guard current == generation else { return }
            result = await fetch()
        }
        guard current == generation, !Task.isCancelled else { return }
        switch result {
        case .success(let links):
            state = .loaded(links)
        case .failure(let error):
            if links == nil { state = .failed(error) }
            toast = ToastCopy.message(for: error)
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .user(what: "me_changed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func dismissToast() {
        toast = nil
    }

    private func fetch() async -> Result<InviteLinks, APIError> {
        do {
            let code = try await api.read { client in
                try await client.getMyReferralCode().ok.body.json
            }
            guard let links = InviteLinks(code) else {
                return .failure(.decoding("invite link is not a URL: \(code.link)"))
            }
            return .success(links)
        } catch {
            return .failure(APIError(error))
        }
    }

    private static func isPending(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error, case .known(.referralCodePending) = problem.code else {
            return false
        }
        return true
    }
}
