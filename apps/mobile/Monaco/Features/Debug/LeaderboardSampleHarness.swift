#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum LeaderboardSampleScenario: String, CaseIterable {
    case cabals
    case people
    case filtered
    case empty
    case loading
    case stale
    case error

    static let launchArgument = "-MonacoLeaderboardSample"

    static func matching(_ arguments: [String]) -> LeaderboardSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return LeaderboardSampleScenario(rawValue: arguments[flag + 1])
    }

    var board: LeaderboardBoardKind { self == .people ? .people : .cabals }

    var filters: LeaderboardFilters {
        var filters = LeaderboardFilters.standard
        if self == .filtered {
            filters.membership = .open
            filters.minimumMembers = .five
        }
        return filters
    }

    var fixture: LeaderboardFixture {
        switch self {
        case .cabals, .people, .filtered: LeaderboardFixture(cabals: Self.cabalEntries(), people: Self.peopleEntries())
        case .empty: LeaderboardFixture()
        case .loading: LeaderboardFixture(phase: .loading)
        case .stale: LeaderboardFixture(cabals: Self.cabalEntries(delayed: true))
        case .error: LeaderboardFixture(phase: .failed)
        }
    }

    private typealias Entry = LeaderboardFixture.Entry
    private typealias Flags = [Components.Schemas.LeaderboardRow.FlagsPayloadPayload]

    private static func cabalEntries(delayed: Bool = false) -> [Entry] {
        let flags: Flags = delayed ? [.stalePrices] : []
        let cabals: [(String, String, Int64, Int64, LeaderboardMovement, Int, LeaderboardMembership)] = [
            ("ape-society", "Ape Society", 12_690_000_000, 4_230, .up(4), 12, .open),
            ("quant-kids", "Quant Kids", 7_420_000_000, 3_710, .down(1), 9, .approval),
            ("dip-buyers", "Dip Buyers", 3_180_000_000, 1_960, .unchanged, 7, .open),
            ("sunday-investors", "Sunday Investors", 1_240_000_000, 880, .up(2), 4, .acceptingRequests),
            ("moon-crew", "Moon Crew", -320_000_000, -210, .down(3), 15, .open),
        ]
        return cabals.enumerated().map { index, cabal in
            Entry(
                row: LeaderboardRowView(
                    .sample(
                        rank: Int32(index + 1), id: cabal.0, name: cabal.1, kind: .cabal,
                        valueMicros: abs(cabal.2) * 3, pnlMicros: cabal.2, returnBps: cabal.3, flags: flags)),
                annotation: LeaderboardAnnotation(movement: cabal.4, memberCount: cabal.5, membership: cabal.6))
        }
    }

    private static func peopleEntries() -> [Entry] {
        let people: [(String, String, String, Int64, Int64, LeaderboardMovement)] = [
            ("ana", "Ana Kovacs", "ana.k", 4_820_000_000, 5_120, .up(2)),
            ("marcus", "Marcus Hale", "marcus", 3_110_000_000, 3_480, .down(1)),
            ("priya", "Priya Nair", "priya.n", 2_240_000_000, 2_050, .unchanged),
            ("theo", "Theo Park", "theo", 910_000_000, 940, .up(1)),
            ("lena", "Lena Ortiz", "lena.o", -140_000_000, -120, .down(2)),
        ]
        return people.enumerated().map { index, person in
            let subject = Components.Schemas.LeaderboardSubject(
                id: person.0, kind: .user, name: person.1, handle: person.2, pictureUrl: nil)
            let row = Components.Schemas.LeaderboardRow(
                rank: Int32(index + 1), subject: subject, valueMicros: abs(person.3) * 2, pnlMicros: person.3,
                returnBps: person.4, flags: [])
            return Entry(row: LeaderboardRowView(row), annotation: LeaderboardAnnotation(movement: person.5))
        }
    }
}

nonisolated struct LeaderboardSampleRoute: AppRoute {
    let scenario: LeaderboardSampleScenario

    @MainActor func destination() -> some View {
        LeaderboardView(
            board: scenario.board, range: .oneMonth, filters: scenario.filters, fixture: scenario.fixture)
    }
}

final class LeaderboardSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = LeaderboardSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(SampleAPIScript())
        return AnyView(SampleAppFrame(auth: auth, tab: .cabals, routes: [LeaderboardSampleRoute(scenario: scenario)]))
    }
}
#endif
