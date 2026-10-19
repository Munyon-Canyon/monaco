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

    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func load(cabalIDs: [String]) async {
        let ids = Array(cabalIDs.prefix(Self.maximumCabals))
        guard !ids.isEmpty else {
            contexts = [:]
            return
        }
        let api = api
        async let members = try? await api.read { try await $0.getMyCabals().ok.body.json }
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
