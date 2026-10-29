import MonacoAPI
import Observation

@Observable
@MainActor
public final class ProfileCabalsModel {
    public private(set) var state: LoadState<[ProfileCabalRow]> = .idle

    private let api: APIClient
    private let hints: any HintSource
    private var generation = 0

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
    }

    public var rows: [ProfileCabalRow] {
        if case .loaded(let rows) = state { return rows }
        return []
    }

    public var memberCount: Int { ProfileCabalRows.memberCount(rows) }

    public func load() async {
        generation += 1
        let issued = generation
        if case .loaded = state {
        } else {
            state = .loading
        }
        let api = api
        async let cabals = Self.attempt { try await api.read { try await $0.getMyCabals().ok.body.json } }
        async let portfolio = Self.attempt { try await api.read { try await $0.getMyPortfolio().ok.body.json } }
        async let invites = Self.attempt { try await api.read { try await $0.getMyCabalInvites().ok.body.json } }
        let (mine, held, invited) = await (cabals, portfolio, invites)
        guard issued == generation else { return }
        switch mine {
        case .success(let cabals):
            state = .loaded(
                ProfileCabalRows.make(
                    cabals: cabals, portfolio: try? held.get(), invites: (try? invited.get()) ?? []))
        case .failure(let error):
            if case .loaded = state { return }
            state = .failed(APIError(error))
        }
    }

    public func observe() async {
        await refresher.observe([
            hints.hints(matching: .user(what: "cabals")),
            hints.hints(matching: .user(what: "balance_changed")),
        ])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    private static func attempt<Value: Sendable>(
        _ call: @Sendable () async throws -> Value
    ) async -> Result<Value, any Error> {
        do { return .success(try await call()) } catch { return .failure(error) }
    }
}
