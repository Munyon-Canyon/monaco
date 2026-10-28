#if DEBUG
import Foundation

extension Components.Schemas.MarketFacts {
    public static let sampleAndres = Self(facts: [
        .init(
            symbol: "NVDAx", fact: .init(kind: .voteOpen, cabalName: "AC Capital", count: 1)),
        .init(symbol: "AAPLx", fact: .init(kind: .held, units: "24.0000", cabalCount: 1)),
        .init(symbol: "TSLAx", fact: .init(kind: .held, units: "2.2000", cabalCount: 1)),
        .init(symbol: "METAx", fact: .init(kind: .called, proposerName: "Christelle")),
        .init(symbol: "AMZNx", fact: .init(kind: .held, units: "5.0000", cabalCount: 1)),
        .init(symbol: "MSFTx", fact: nil),
        .init(symbol: "tSpaceX", fact: nil),
    ])

    public static let sampleEmpty = Self(facts: [])
}
#endif
