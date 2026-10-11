import Foundation
import MonacoAPI

public struct BestPrice: Equatable, Sendable {
    public struct Venue: Equatable, Sendable, Identifiable {
        public let id: String
        public let name: String
        public let network: Components.Schemas.Network
        public let netShareNanos: Int64
        public let netMicros: Int64
        public let isWinner: Bool
    }

    public struct UnavailableVenue: Equatable, Sendable, Identifiable {
        public let id: String
        public let name: String
        public let network: Components.Schemas.Network
        public let reason: String
    }

    public let venues: [Venue]
    public let unavailable: [UnavailableVenue]
    public let isSell: Bool

    public init?(_ comparison: Components.Schemas.PriceComparison?, isSell: Bool) {
        guard let comparison else { return nil }
        let venues = comparison.quotes.compactMap { quote -> Venue? in
            guard let shares = Int64(quote.netShareNanos), let micros = Int64(quote.netMicros) else { return nil }
            return Venue(
                id: quote.listingId, name: Self.venueName(venue: quote.venue, network: quote.network),
                network: quote.network, netShareNanos: shares, netMicros: micros,
                isWinner: quote.listingId == comparison.winnerListingId)
        }
        guard let winner = venues.first(where: \.isWinner) else { return nil }
        self.venues = [winner] + venues.filter { !$0.isWinner }
        unavailable = comparison.unavailable.compactMap { venue -> UnavailableVenue? in
            guard let exchange = Components.Schemas.RouteQuote.VenuePayload(rawValue: venue.venue.rawValue) else {
                return nil
            }
            return UnavailableVenue(
                id: venue.listingId, name: Self.venueName(venue: exchange, network: venue.network),
                network: venue.network, reason: Self.reasonText(venue.reason))
        }
        self.isSell = isSell
    }

    public var winner: Venue { venues[0] }

    public var headline: String {
        venues.count == 1 ? "Best price across 1 venue" : "Best price across \(venues.count) venues"
    }

    public var price: String? {
        guard winner.netShareNanos > 0 else { return nil }
        let (priceMicros, _) = winner.netShareNanos.dividingFullWidth(
            winner.netMicros.multipliedFullWidth(by: 1_000_000_000))
        return "\(UsdAmountFormatter.format(micros: priceMicros)) a share"
    }

    public func figure(_ venue: Venue) -> String {
        isSell ? UsdAmountFormatter.format(micros: venue.netMicros) : Self.shares(nanos: venue.netShareNanos)
    }

    public var advantage: String? {
        guard venues.count > 1 else { return nil }
        let runnerUp = venues[1]
        if isSell {
            let more = winner.netMicros - runnerUp.netMicros
            guard more > 0 else { return nil }
            return "You get \(UsdAmountFormatter.format(micros: more)) more than the next best"
        }
        let more = winner.netShareNanos - runnerUp.netShareNanos
        guard more >= Self.nanosPerTenThousandth else { return nil }
        return "You get \(Self.number(nanos: more)) more shares than the next best"
    }

    static let nanosPerTenThousandth: Int64 = 100_000

    static func reasonText(_ reason: Components.Schemas.UnavailableVenue.ReasonPayload) -> String {
        switch reason {
        case .noRoute: "No route"
        case .stale, .expired, .simulationFailed, .impact: "Unavailable right now"
        }
    }

    static func shares(nanos: Int64) -> String {
        "\(number(nanos: nanos)) shares"
    }

    private static func number(nanos: Int64) -> String {
        let tenThousandths = nanos / nanosPerTenThousandth
        let fraction = String(tenThousandths % 10_000)
        return "\(tenThousandths / 10_000).\(String(repeating: "0", count: 4 - fraction.count))\(fraction)"
    }

    static func venueName(
        venue: Components.Schemas.RouteQuote.VenuePayload, network: Components.Schemas.Network
    ) -> String {
        let exchange =
            switch venue {
            case .jupiter: "Jupiter"
            case .uniswap: "Uniswap"
            }
        let chain =
            switch network {
            case .solana: "Solana"
            case .base: "Base"
            case .arbitrum: "Arbitrum"
            case .ethereum: "Ethereum"
            case .robinhood: "Robinhood Chain"
            }
        return "\(exchange) on \(chain)"
    }
}
