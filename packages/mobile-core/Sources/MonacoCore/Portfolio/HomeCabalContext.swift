import MonacoAPI
import Observation

public struct HomeCabalContext: Equatable, Sendable {
    public let memberCount: Int?
    public let ownershipBps: Int64?

    public init(memberCount: Int?, ownershipBps: Int64?) {
        self.memberCount = memberCount
        self.ownershipBps = ownershipBps
    }

    public var memberCountText: String {
        guard let memberCount else { return "Cabal" }
        return memberCount == 1 ? "1 member" : "\(memberCount) members"
    }

    public var ownershipText: String? {
        ownershipBps.map { PercentFormatter.format(basisPoints: $0, signed: false, fractionDigits: 2) }
    }
}

@Observable
@MainActor
public final class HomeCabalContextModel {
    public static let maximumCabals = 12

    public private(set) var contexts: [String: HomeCabalContext] = [:]
    public private(set) var memberships: [Components.Schemas.MyCabal]?

    private let api: APIClient
    private var membershipsRead = false

    public init(api: APIClient) {
        self.api = api
    }

    private static func members(known: [Components.Schemas.MyCabal]?, reading: Bool, api: APIClient) async
        -> [Components.Schemas.MyCabal]?
    {
        guard reading else { return known }
        return try? await api.read { try await $0.getMyCabals().ok.body.json }
    }

    public func loadMemberships(refreshing: Bool = false) async {
        guard refreshing || !membershipsRead else { return }
        let fetched = try? await api.read { try await $0.getMyCabals().ok.body.json }
        guard !Task.isCancelled else { return }
        if let fetched { memberships = fetched }
        membershipsRead = true
    }

    public func observe(hints: any HintSource) async {
        for await _ in hints.hints(matching: .user(what: "cabals")) {
            await loadMemberships(refreshing: true)
        }
    }

    public func load(cabalIDs: [String]) async {
        let ids = Array(cabalIDs.prefix(Self.maximumCabals))
        guard !ids.isEmpty else {
            contexts = [:]
            return
        }
        let api = api
        let known = membershipsRead ? memberships : nil
        let reads = !membershipsRead
        async let members = Self.members(known: known, reading: reads, api: api)
        let ownership = await withTaskGroup(of: (String, Int64?).self) { group in
            for id in ids {
                group.addTask {
                    let pot = try? await api.read { try await $0.getCabalPot(path: .init(id: id)).ok.body.json }
                    return (id, pot?.me.map { Int64($0.sliceBps) })
                }
            }
            var found: [String: Int64] = [:]
            for await (id, bps) in group {
                if let bps { found[id] = bps }
            }
            return found
        }
        let counts = Dictionary(
            (await members ?? []).map { ($0.id, Int($0.memberCount)) }, uniquingKeysWith: { first, _ in first })
        guard !Task.isCancelled else { return }
        contexts = Dictionary(
            uniqueKeysWithValues: ids.map {
                ($0, HomeCabalContext(memberCount: counts[$0], ownershipBps: ownership[$0]))
            })
    }
}
