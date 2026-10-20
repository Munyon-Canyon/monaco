import Foundation
import MonacoAPI

public struct HomeCabalItem: Equatable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let pictureURL: String?
    public let memberCount: Int?
    public let standing: PortfolioSummary.Row?

    public var isFunded: Bool { (standing?.valueMicros ?? 0) > 0 }
}

public enum HomeCabalsState: Equatable, Sendable {
    case loading
    case empty
    case cabals([HomeCabalItem])
}

public enum HomeCabalList {
    public static func state(portfolio: PortfolioSummary, memberships: [Components.Schemas.MyCabal]?)
        -> HomeCabalsState
    {
        let items = items(portfolio: portfolio, memberships: memberships)
        if !items.isEmpty { return .cabals(items) }
        return memberships == nil ? .loading : .empty
    }

    public static func items(portfolio: PortfolioSummary, memberships: [Components.Schemas.MyCabal]?)
        -> [HomeCabalItem]
    {
        guard let memberships else {
            return portfolio.rows.map {
                HomeCabalItem(id: $0.id, name: $0.name, pictureURL: $0.pictureURL, memberCount: nil, standing: $0)
            }
        }
        let byID = Dictionary(memberships.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let funded = portfolio.rows.filter { $0.valueMicros > 0 }.map { row in
            HomeCabalItem(
                id: row.id, name: row.name, pictureURL: row.pictureURL,
                memberCount: byID[row.id].map { Int($0.memberCount) }, standing: row)
        }
        let fundedIDs = Set(funded.map(\.id))
        let standings = Dictionary(portfolio.rows.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let unfunded = byID.values.filter { !fundedIDs.contains($0.id) }
            .sorted { ($0.joinedAt, $0.id) > ($1.joinedAt, $1.id) }
            .map { cabal in
                HomeCabalItem(
                    id: cabal.id, name: cabal.name, pictureURL: cabal.pictureUrl, memberCount: Int(cabal.memberCount),
                    standing: standings[cabal.id])
            }
        return funded + unfunded
    }
}
