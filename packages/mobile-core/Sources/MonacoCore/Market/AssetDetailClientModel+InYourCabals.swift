import Foundation
import MonacoAPI

extension AssetDetailClientModel {
    public func loadInYourCabals() async {
        async let held: Void = loadHeld()
        async let proposals: Void = loadOpenProposals()
        await held
        await proposals
    }

    func loadHeld() async {
        guard let api else { return }
        let symbol = symbol
        guard
            let list = try? await api.read({
                try await $0.getMyHeld(query: .init(symbol: symbol)).ok.body.json
            })
        else { return }
        let cabals = try? await api.read({ try await $0.getMyCabals().ok.body.json })
        let votable = Dictionary(
            uniqueKeysWithValues: positions.map { ($0.cabalID, $0.canVote) })
        let voting = cabals.map { Dictionary(uniqueKeysWithValues: $0.map { ($0.id, $0.canVote) }) }
        let rows = list.holdings.filter { $0.holding.symbol.caseInsensitiveCompare(symbol) == .orderedSame }
        positions = rows.map { row in
            let holding = row.holding
            return AssetCabalPosition(
                cabalID: row.cabal.id, cabalName: row.cabal.name, pictureURL: row.cabal.pictureUrl,
                canVote: (voting ?? votable)[row.cabal.id] ?? false, units: holding.units,
                kind: AssetKind(raw: holding.kind.rawValue), valueMicros: holding.valueMicros,
                pnlMicros: holding.pnlMicros, costBasisMicros: holding.costBasisMicros)
        }.sorted {
            $0.valueMicros == $1.valueMicros ? $0.cabalID < $1.cabalID : $0.valueMicros > $1.valueMicros
        }
        let order = Dictionary(uniqueKeysWithValues: positions.enumerated().map { ($1.cabalID, $0) })
        heldRows = rows.map(AssetHeldRowFacts.init).sorted { (order[$0.cabalID] ?? 0) < (order[$1.cabalID] ?? 0) }
        let ids = Set(positions.map(\.cabalID))
        if ids != heldCabalIDs {
            heldCabalIDs = ids
            heldCabalIDChanges?.yield(ids)
        }
    }

    func loadOpenProposals() async {
        guard let api else { return }
        let symbol = symbol
        var items: [Components.Schemas.FeedItem] = []
        var cursor: String?
        repeat {
            let after = cursor
            guard
                let page = try? await api.read({
                    try await $0.getFeed(
                        query: .init(
                            kind: "proposal", symbol: symbol, scope: .mine, status: .open, cursor: after,
                            limit: 20)
                    ).ok.body.json
                })
            else { return }
            items += page.items
            cursor = page.nextCursor == after ? nil : page.nextCursor
        } while cursor != nil
        let now = now()
        openProposals = items.compactMap { AssetOpenProposalRowFacts($0, now: now) }
    }

    func observeHeldActivity(_ hints: any HintSource) async {
        let (changes, continuation) = AsyncStream.makeStream(of: Set<String>.self)
        heldCabalIDChanges = continuation
        continuation.yield(heldCabalIDs)
        var current: Task<Void, Never>?
        for await ids in changes {
            current?.cancel()
            let streams = ids.sorted().map { hints.hints(matching: .cabal(id: $0, what: "activity_changed")) }
            current = Task { await self.activityRefresher.observe(streams) }
        }
        current?.cancel()
        heldCabalIDChanges = nil
    }
}
