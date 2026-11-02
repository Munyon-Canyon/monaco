import Foundation

public enum ProposeMath {
    public static func tokenAmount(units: UInt128, dollars: Int64, holdingValue: Int64) -> UInt128 {
        guard units > 0, dollars > 0, holdingValue > 0 else { return 0 }
        guard dollars < holdingValue else { return units }
        let product = units.multipliedReportingOverflow(by: UInt128(dollars))
        if product.overflow { return units / UInt128(holdingValue) * UInt128(dollars) }
        return product.partialValue / UInt128(holdingValue)
    }

    public static func quantityLabel(kind: AssetKind) -> String {
        kind == .preIpo ? "tokens" : "shares"
    }
}
