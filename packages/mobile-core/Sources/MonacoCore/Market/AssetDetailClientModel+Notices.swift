import Foundation

extension AssetDetailClientModel {
    public var notices: [AssetNotice] {
        guard let detail else { return [] }
        var found: [AssetNotice] = []
        if !detail.isTradable {
            found.append(.unavailable)
        } else if !detail.isQuotable {
            found.append(.paused)
        }
        return found
    }
}
