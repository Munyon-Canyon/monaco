import Foundation

extension AssetDetailClientModel {
    public var staleMinutes: Int? {
        guard let detail, detail.priceMicros != nil else { return nil }
        let current = now()
        if let asOf = detail.priceAsOf, current.timeIntervalSince(asOf) > MarketStaleness.priceAgeLimit {
            return MarketStaleness.minutes(from: asOf, to: current)
        }
        guard refreshFailed, let lastLoadedAt else { return nil }
        return MarketStaleness.minutes(from: lastLoadedAt, to: current)
    }

    public var notices: [AssetNotice] {
        guard let detail else { return [] }
        var found: [AssetNotice] = []
        if let minutes = staleMinutes { found.append(.stale(minutes: minutes)) }
        if !detail.isTradable {
            found.append(.unavailable)
        } else if !detail.isQuotable {
            found.append(.paused)
        }
        return found
    }
}
