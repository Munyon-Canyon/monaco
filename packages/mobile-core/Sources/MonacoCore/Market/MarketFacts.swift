import MonacoAPI

public enum MarketFact: Equatable, Sendable {
    case voteOpen(cabalName: String, count: Int)
    case held(units: String)
    case called(proposerName: String)
}

public enum MarketFacts {
    public static func map(_ wire: Components.Schemas.MarketFacts) -> [String: MarketFact] {
        var facts: [String: MarketFact] = [:]
        for row in wire.facts {
            if let fact = row.fact.flatMap(fact) { facts[row.symbol] = fact }
        }
        return facts
    }

    public static func subline(_ fact: MarketFact?, ticker: String) -> String {
        switch fact {
        case .voteOpen(let cabalName, let count):
            count > 1 ? "\(count) votes open" : "Vote open in \(cabalName)"
        case .held(let units):
            "\(trimmed(units)) sh held"
        case .called(let proposerName):
            ResultCopy.calledIt(proposerName)
        case nil:
            ticker
        }
    }

    static func trimmed(_ units: String) -> String {
        guard units.contains(".") else { return units }
        var text = units
        while text.hasSuffix("0") { text.removeLast() }
        if text.hasSuffix(".") { text.removeLast() }
        return text.isEmpty ? "0" : text
    }

    private static func fact(_ wire: Components.Schemas.MarketFactRow.FactPayload) -> MarketFact? {
        switch wire.kind {
        case .voteOpen:
            guard let cabalName = wire.cabalName else { return nil }
            return .voteOpen(cabalName: cabalName, count: wire.count ?? 1)
        case .held:
            return wire.units.map { .held(units: $0) }
        case .called:
            return wire.proposerName.map { .called(proposerName: $0) }
        }
    }
}
