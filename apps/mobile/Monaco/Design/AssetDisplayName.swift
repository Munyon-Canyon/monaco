import Foundation
import MonacoCore

/// Strips xStocks catalog branding from user-facing asset names.
enum AssetDisplayName {
    static func format(catalogName: String, kind: AssetKind = .stock) -> String {
        CatalogAssetNameFormatter.format(catalogName, kind: kind)
    }
}
