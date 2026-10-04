import MonacoAPI
import SwiftUI

struct CabalPortfolioRow: View {
    let cabal: Components.Schemas.MyCabal
    var isLast = false

    var body: some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: cabal.id))) {
            MonacoRow(title: cabal.name, subtitle: Self.members(cabal.memberCount), isLast: isLast) {
                CabalMark(groupId: cabal.id, name: cabal.name, size: 40, pictureUrl: cabal.pictureUrl)
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("cabal-row-\(cabal.id)")
    }

    static func members(_ count: Int32) -> String {
        count == 1 ? "1 member" : "\(count) members"
    }
}
