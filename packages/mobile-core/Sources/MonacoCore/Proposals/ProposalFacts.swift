import Foundation

public struct ProposalFacts: Equatable, Sendable {
    public enum Info: Equatable, Sendable { case price, estimate }

    public let estimate: String?
    public let info: Info?
    public let treasury: String?

    public var spoken: String {
        [estimate, treasury].compactMap { $0 }
            .map { line in
                let plain = line.replacingOccurrences(of: "<0.1%", with: "Less than 0.1 percent")
                return "\(plain.replacingOccurrences(of: "%", with: " percent"))."
            }
            .joined(separator: " ")
    }

    public static func make(detail: ProposalDetail, asset: ProposalAsset?) -> ProposalFacts? {
        let summary = detail.summary
        let stats = detail.stats
        if summary.isRule { return nil }
        if let swap = summary.swap, swap.status == "confirmed", swap.filledAt != nil, swap.priceMicros != nil {
            return nil
        }
        let isSell = summary.kind == "sell"
        let estimateMicros = asset?.kind == .preIpo ? asset?.estimateMicros : nil
        let projected = estimateMicros.flatMap {
            ProposalStats.estimatedSharesFigure(usdcMicros: isSell ? nil : summary.usdcMicros, estimateMicros: $0)
        }
        let exact = projected == nil && (stats.sharesExact || isSell)
        let shares = present(projected ?? ProposalStats.sharesText(stats, exact: true))
        let price = present(ProposalStats.money(estimateMicros ?? stats.priceMicros))
        let treasury = stats.treasuryShareBps.map { "\(ProposalStats.percent($0)) of treasury" }
        let line = sentence(shares: shares, price: price, exact: exact, estimated: estimateMicros != nil)
        return ProposalFacts(
            estimate: line, info: line == nil ? nil : estimateMicros == nil ? .price : .estimate,
            treasury: treasury)
    }

    private static func present(_ figure: String?) -> String? {
        figure == ProposalStats.missing ? nil : figure
    }

    private static func sentence(shares: String?, price: String?, exact: Bool, estimated: Bool) -> String? {
        let priced = price.map { estimated ? "an estimated \($0)" : $0 }
        guard let shares else { return price.map { estimated ? "An estimated \($0) a share" : "\($0) a share" } }
        let unit = ["1", "1.00"].contains(shares) ? "share" : "shares"
        let counted = "\(exact ? "" : "About ")\(shares) \(unit)"
        return priced.map { "\(counted) at \($0)" } ?? counted
    }
}
