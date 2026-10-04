import SwiftUI

enum CabalPotSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalInkBand {
            Text("In the pot")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.onHero)
                .accessibilityAddTraits(.isHeader)
            CabalInkCaption("The pot's value shows up here soon.", id: "cabal-pot-coming")
        }
    }
}

struct CabalInkBand<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            content
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background {
            MonacoTheme.heroInk.padding(.top, -MonacoTheme.Space.gutter)
        }
    }
}

struct CabalInkCaption: View {
    let text: String
    let id: String

    init(_ text: String, id: String) {
        self.text = text
        self.id = id
    }

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.onHeroMuted)
            .accessibilityIdentifier(id)
    }
}
