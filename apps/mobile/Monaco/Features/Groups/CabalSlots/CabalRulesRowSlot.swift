import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalRulesRowSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalRulesRow(cabalID: context.cabalID)
    }
}

private struct CabalRulesRow: View {
    let cabalID: String
    @Environment(\.cabalModel) private var cabalModel

    var body: some View {
        if let cabal = cabalModel?.cabal, cabal.me != nil {
            NavigationLink(value: AnyAppRoute(CabalInfoRoute(cabalID: cabalID))) {
                MonacoRow(
                    title: "Rules & members", subtitle: CabalCopy.memberCount(cabal.memberCount), chevron: true,
                    isLast: true
                ) {
                    SunkenGlyphMark(icon: .info)
                }
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("cabal-rules-row")
        } else {
            Color.clear.frame(height: 0)
        }
    }
}
