import MonacoAPI
import SwiftUI

enum FeedResultBandMetrics {
    static let height: CGFloat = 72
}

struct FeedResultBand: View {
    let cabalID: String
    let pictureURL: String?

    var body: some View {
        CabalCover(
            cabalId: cabalID, tint: .forGroupId(cabalID), pictureURL: Self.resolved(pictureURL), style: .art
        ) { EmptyView() }
        .frame(maxWidth: .infinity)
        .frame(height: FeedResultBandMetrics.height)
        .accessibilityHidden(true)
    }

    private static func resolved(_ raw: String?) -> URL? {
        guard let raw = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else { return nil }
        return URL(string: raw)
    }
}

extension FeedResultBand {
    init?(_ item: Components.Schemas.FeedItem) {
        guard item.kind == "result", item.shape == .cabal, let cabalID = item.cabalId else { return nil }
        self.init(cabalID: cabalID, pictureURL: item.cabalPictureUrl)
    }
}
