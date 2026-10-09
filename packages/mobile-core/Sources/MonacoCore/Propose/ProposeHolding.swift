import MonacoAPI

public struct ProposeHolding: Equatable, Hashable, Sendable, Identifiable {
    public let symbol: String
    public let ticker: String
    public let name: String
    public let kind: AssetKind
    public let tokenAmount: Int64
    public let priceMicros: Int64
    public let valueMicros: Int64
    private let unitsTenThousandths: Int64

    public var id: String { symbol }

    public init(_ holding: Components.Schemas.CabalHolding) {
        let kind = AssetKind(raw: holding.kind.rawValue)
        symbol = holding.symbol
        ticker = AssetSymbolFormatter.display(holding.symbol, kind: kind)
        name = holding.displayName
        self.kind = kind
        tokenAmount = holding.tokenAmount
        priceMicros = holding.priceMicros
        valueMicros = holding.valueMicros
        unitsTenThousandths = Int64(holding.units.replacingOccurrences(of: ".", with: "")) ?? 0
    }

    public var detail: String {
        "\(name) · \(quantity(of: tokenAmount))"
    }

    public static let noPriceHelper = "No price for this stock right now"

    public var helperText: String {
        guard valueMicros > 0 else { return Self.noPriceHelper }
        return "The cabal holds \(quantity(of: tokenAmount)) · \(UsdAmountFormatter.format(micros: valueMicros))"
    }

    public func tokenAmount(forMicros micros: Int64) -> Int64 {
        guard micros > 0 else { return 0 }
        if micros >= valueMicros - valueMicros % 10_000 { return tokenAmount }
        return ProposeMath.tokenAmount(units: tokenAmount, dollars: micros, holdingValue: valueMicros)
    }

    public func quantity(of amount: Int64) -> String {
        let clamped = min(max(amount, 0), tokenAmount)
        let scaled =
            tokenAmount > 0
            ? tokenAmount.dividingFullWidth(unitsTenThousandths.multipliedFullWidth(by: clamped)).quotient : 0
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
