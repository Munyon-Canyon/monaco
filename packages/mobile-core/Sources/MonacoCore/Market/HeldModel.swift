import Foundation
import MonacoAPI
import Observation

public struct HeldRowFacts: Equatable, Sendable, Identifiable {
    public let cabalID: String
    public let symbol: String
    public let kind: AssetKind
    public let title: String
    public let subtitle: String
    public let logoURL: URL?
    public let valueMicros: Int64
    public let returnText: String?
    public let valueNote: String?

    public var id: String { cabalID + "/" + symbol }
    public var valueText: String { UsdAmountFormatter.format(micros: valueMicros) }

    public init(_ row: Components.Schemas.HeldRow) {
        let holding = row.holding
        let isPreIpo = holding.kind == .preIpo
        cabalID = row.cabal.id
        symbol = holding.symbol
        kind = AssetKind(raw: holding.kind.rawValue)
        title = holding.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        subtitle =
            "\(row.cabal.name) · \(Self.shares(holding.units)) sh" + (isPreIpo ? " · Pre-IPO" : "")
        logoURL = row.logoUrl.flatMap(URL.init(string:))
        valueMicros = holding.valueMicros
        valueNote = isPreIpo ? PreIpoCopy.heldAtEstimate : nil
        let (scaled, overflow) = holding.pnlMicros.multipliedReportingOverflow(by: 10_000)
        returnText =
            holding.costBasisMicros > 0 && !overflow && !isPreIpo
            ? PercentFormatter.format(
                basisPoints: scaled / holding.costBasisMicros, signed: true, fractionDigits: 1)
            : nil
    }

    static func shares(_ units: String) -> String {
        var shares = Substring(units)
        guard shares.contains(".") else { return String(shares) }
        while shares.last == "0" { shares = shares.dropLast() }
        if shares.last == "." { shares = shares.dropLast() }
        return String(shares)
    }
}

public struct HeldList: Equatable, Sendable {
    public let rows: [HeldRowFacts]
    public let cabalIDs: Set<String>
    public let totalMicros: Int64
    public let cabalCount: Int

    public init(_ list: Components.Schemas.HeldList) {
        rows = list.holdings.map(HeldRowFacts.init)
        cabalIDs = Set(list.holdings.map(\.cabal.id))
        totalMicros = list.totalValueMicros
        cabalCount = Int(list.cabalCount)
    }

    public var totalLine: String {
        "\(UsdAmountFormatter.format(micros: totalMicros)) across \(cabalCount) \(cabalCount == 1 ? "cabal" : "cabals")"
    }
}

@Observable
@MainActor
public final class HeldModel {
    public private(set) var state: LoadState<HeldList> = .idle
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    private let api: APIClient
    private let hints: any HintSource
    private let clock: any Clock<Duration>
    private var generation = 0
    private var settled = 0
    private var cabalIDs: Set<String> = []
    @ObservationIgnored private var cabalIDChanges: AsyncStream<Set<String>>.Continuation?
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource, clock: any Clock<Duration>) {
        self.api = api
        self.hints = hints
        self.clock = clock
    }

    public func load() async {
        generation += 1
        let issued = generation
        if case .loaded = state {} else { state = .loading }
        do {
            let list = HeldList(try await api.read { try await $0.getMyHeld().ok.body.json })
            guard issued > settled else { return }
            settled = issued
            state = .loaded(list)
            lastError = nil
            if list.cabalIDs != cabalIDs {
                cabalIDs = list.cabalIDs
                cabalIDChanges?.yield(list.cabalIDs)
            }
        } catch {
            if Task.isCancelled { return }
            guard issued > settled else { return }
            settled = issued
            let error = APIError(error)
            if BackgroundRefresh.isActive, case .loaded = state { return }
            lastError = error
            failureTick += 1
            if case .loaded = state { return }
            state = .failed(error)
        }
    }

    public func observe() async {
        let (changes, continuation) = AsyncStream.makeStream(of: Set<String>.self)
        cabalIDChanges = continuation
        continuation.yield(cabalIDs)
        var current: Task<Void, Never>?
        for await ids in changes {
            current?.cancel()
            let streams =
                [
                    hints.hints(matching: .global(what: "prices_updated")),
                    hints.hints(matching: .user(what: "cabals")),
                ] + ids.sorted().map { hints.hints(matching: .cabal(id: $0, what: "activity_changed")) }
            current = Task { await self.refresher.observe(streams) }
        }
        current?.cancel()
        cabalIDChanges = nil
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
