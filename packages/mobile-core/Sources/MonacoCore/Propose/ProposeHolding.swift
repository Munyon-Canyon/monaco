import MonacoAPI

public struct ProposeHolding: Equatable, Hashable, Sendable, Identifiable {
    public struct Location: Equatable, Hashable, Sendable, Identifiable {
        public let network: Components.Schemas.Network
        public let tokenAmount: Int64
        public let valueMicros: Int64
        public let exactTokens: UInt128

        public var overflows: Bool { exactTokens > UInt128(Int64.max) }

        public var id: String { network.rawValue }
    }

    public let symbol: String
    public let ticker: String
    public let name: String
    public let kind: AssetKind
    public let tokenAmount: Int64
    public let priceMicros: Int64
    public let valueMicros: Int64
    public let exactTokens: UInt128
    public let network: Components.Schemas.Network?
    public let locations: [Location]
    private let unitsTenThousandths: Int64

    public var id: String { symbol }

    public init(_ holding: Components.Schemas.CabalHolding) {
        let kind = AssetKind(raw: holding.kind.rawValue)
        symbol = holding.symbol
        ticker = AssetSymbolFormatter.display(holding.symbol, kind: kind)
        name = holding.displayName
        self.kind = kind
        priceMicros = holding.priceMicros
        let locations = Self.locations(of: holding)
        self.locations = locations
        guard locations.contains(where: { $0.network != .solana }),
            let first = locations.first(where: { $0.network == .solana }) ?? locations.first
        else {
            network = nil
            tokenAmount = holding.tokenAmount
            exactTokens = UInt128(clamping: holding.tokenAmount)
            valueMicros = holding.valueMicros
            unitsTenThousandths = Int64(holding.units.replacingOccurrences(of: ".", with: "")) ?? 0
            return
        }
        network = first.network
        tokenAmount = first.tokenAmount
        exactTokens = first.exactTokens
        valueMicros = first.valueMicros
        unitsTenThousandths = Self.tenThousandths(valueMicros: first.valueMicros, priceMicros: holding.priceMicros)
    }

    private init(_ holding: ProposeHolding, at location: Location) {
        symbol = holding.symbol
        ticker = holding.ticker
        name = holding.name
        kind = holding.kind
        priceMicros = holding.priceMicros
        locations = holding.locations
        network = location.network
        tokenAmount = location.tokenAmount
        exactTokens = location.exactTokens
        valueMicros = location.valueMicros
        unitsTenThousandths = Self.tenThousandths(valueMicros: location.valueMicros, priceMicros: holding.priceMicros)
    }

    public func at(_ network: Components.Schemas.Network) -> ProposeHolding {
        locations.first { $0.network == network }.map { ProposeHolding(self, at: $0) } ?? self
    }

    private static func locations(of holding: Components.Schemas.CabalHolding) -> [Location] {
        var found: [Location] = (holding.listings ?? []).compactMap { listing in
            guard let exact = UInt128(listing.tokenAmount), exact > 0 else { return nil }
            return Location(
                network: listing.network, tokenAmount: Int64(listing.tokenAmount) ?? Int64.max,
                valueMicros: listing.valueMicros, exactTokens: exact)
        }
        if holding.tokenAmount > 0, !found.contains(where: { $0.network == .solana }) {
            let elsewhere = found.reduce(Int64(0)) { $0 + $1.valueMicros }
            found.insert(
                Location(
                    network: .solana, tokenAmount: holding.tokenAmount,
                    valueMicros: max(0, holding.valueMicros - elsewhere),
                    exactTokens: UInt128(clamping: holding.tokenAmount)),
                at: 0)
        }
        return found
    }

    private static func tenThousandths(valueMicros: Int64, priceMicros: Int64) -> Int64 {
        guard valueMicros > 0, priceMicros > 0 else { return 0 }
        return priceMicros.dividingFullWidth(valueMicros.multipliedFullWidth(by: 10_000)).quotient
    }

    public var detail: String {
        "\(name) · \(heldQuantity)"
    }

    public static let noPriceHelper = "No price for this stock right now"

    public var helperText: String {
        guard valueMicros > 0 else { return Self.noPriceHelper }
        let held = overflows ? heldQuantity : quantity(of: tokenAmount)
        return "The cabal holds \(held) · \(UsdAmountFormatter.format(micros: valueMicros))"
    }

    public var overflows: Bool { exactTokens > UInt128(Int64.max) }

    public func exceedsSettleLimit(forMicros micros: Int64) -> Bool {
        overflows && exactTokens(forMicros: micros) > UInt128(Int64.max)
    }

    private func exactTokens(forMicros micros: Int64) -> UInt128 {
        guard micros > 0, valueMicros > 0 else { return 0 }
        if micros >= valueMicros - valueMicros % 10_000 { return exactTokens }
        let product = exactTokens.multipliedReportingOverflow(by: UInt128(micros))
        if product.overflow { return exactTokens / UInt128(valueMicros) * UInt128(micros) }
        return product.partialValue / UInt128(valueMicros)
    }

    public func tokenAmount(forMicros micros: Int64) -> Int64 {
        guard micros > 0 else { return 0 }
        if overflows {
            let exact = exactTokens(forMicros: micros)
            return Int64(clamping: exact)
        }
        if micros >= valueMicros - valueMicros % 10_000 { return tokenAmount }
        return ProposeMath.tokenAmount(units: tokenAmount, dollars: micros, holdingValue: valueMicros)
    }

    public var heldQuantity: String {
        guard locations.count > 1 || overflows else { return quantity(of: tokenAmount) }
        let total = locations.reduce(Int64(0)) {
            $0 + Self.tenThousandths(valueMicros: $1.valueMicros, priceMicros: priceMicros)
        }
        return Self.label(tenThousandths: total, kind: kind)
    }

    public func quantity(of amount: Int64) -> String {
        if overflows {
            let scaled = UInt128(max(unitsTenThousandths, 0)) * UInt128(max(amount, 0)) / exactTokens
            return Self.label(tenThousandths: min(Int64(clamping: scaled), unitsTenThousandths), kind: kind)
        }
        let clamped = min(max(amount, 0), tokenAmount)
        let scaled =
            tokenAmount > 0
            ? tokenAmount.dividingFullWidth(unitsTenThousandths.multipliedFullWidth(by: clamped)).quotient : 0
        return Self.label(tenThousandths: scaled, kind: kind)
    }

    private static func label(tenThousandths scaled: Int64, kind: AssetKind) -> String {
        var text = "\(scaled / 10_000)"
        var fraction = String(scaled % 10_000)
        fraction = String(repeating: "0", count: 4 - fraction.count) + fraction
        while fraction.last == "0" { fraction.removeLast() }
        if !fraction.isEmpty { text += ".\(fraction)" }
        let noun = ProposeMath.quantityLabel(kind: kind)
        return text == "1" ? "1 \(noun.dropLast())" : "\(text) \(noun)"
    }

    public static func chooserDetail(_ holdings: [ProposeHolding]) -> String {
        let names = holdings.map(\.name)
        switch names.count {
        case 0: return "Nothing to sell yet"
        case 1: return names[0]
        case 2: return "\(names[0]) and \(names[1])"
        default: return "\(names[0]), \(names[1]) and \(names.count - 2) more"
        }
    }
}

public enum ProposeTrade: Equatable, Hashable, Sendable {
    case buy(symbol: String, kind: AssetKind, tokenDecimals: Int?)
    case sell(ProposeHolding)

    public var isSell: Bool {
        if case .sell = self { return true }
        return false
    }

    public var symbol: String {
        switch self {
        case .buy(let symbol, _, _): symbol
        case .sell(let holding): holding.symbol
        }
    }
}
