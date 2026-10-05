import Foundation

public enum ProposeMath {
    public static func tokenAmount(units: Int64, dollars: Int64, holdingValue: Int64) -> Int64 {
        guard units > 0, dollars > 0, holdingValue > 0 else { return 0 }
        guard dollars < holdingValue else { return units }
        return holdingValue.dividingFullWidth(units.multipliedFullWidth(by: dollars)).quotient
    }

    public static func quantityLabel(kind: AssetKind) -> String {
        kind == .preIpo ? "tokens" : "shares"
    }
}
