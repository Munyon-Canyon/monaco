#if DEBUG
import SwiftUI

struct GalleryTypePage: View {
    private enum Sample {
        case text(String, Font)
        case figure(String, String, MoneyStyle)
        case percent(Int64, MoneyStyle)
    }

    private struct Role {
        let name: String
        let spec: String
        let use: String
        let sample: Sample
    }

    private let roles: [Role] = [
        Role(
            name: "Large Title", spec: "28 / 34 bold",
            use: "Root tab titles", sample: .text("Markets", MonacoTheme.Typo.display)),
        Role(
            name: "Title 1", spec: "28 / 34 bold",
            use: "Moments", sample: .text("You're in.", MonacoTheme.Typo.title)),
        Role(
            name: "Title 2", spec: "22 / 28 bold", use: "Name on a pushed screen (cabal, stock, proposal)",
            sample: .text("AC Capital Management", MonacoTheme.Typo.title2)),
        Role(
            name: "Display figure", spec: "28 / 34 heavy, \u{2212}0.5",
            use: "The one hero number a screen: net worth, treasury, price",
            sample: .figure("$20,000", ".00", .display)),
        Role(
            name: "Section", spec: "17 / 22 semibold", use: "Section and sheet titles (was Title 2 / 3)",
            sample: .text("Your cabals", MonacoTheme.Typo.section)),
        Role(
            name: "Button", spec: "16 / 21 semibold",
            use: "Buttons", sample: .text("Propose buy", MonacoTheme.Typo.button)),
        Role(
            name: "Figure", spec: "20 / 25 semibold",
            use: "A figure inside a tile or card: mover percentage, Pre-IPO valuation, a feed card's headline",
            sample: .percent(790, .card)),
        Role(
            name: "Rank", spec: "15 / 20 semibold", use: "Rank numerals in rows and on the podium, tabular",
            sample: .text("4", MonacoTheme.Typo.rank)),
        Role(
            name: "Row title", spec: "15 / 20 semibold", use: "Row, post, comment and tile titles; row values",
            sample: .text("Buy NVDA · $400", MonacoTheme.Typo.headline)),
        Role(
            name: "Content", spec: "15 / 20 regular", use: "Thesis, comments, sheet and empty-state copy",
            sample: .text("Small add to the position.", MonacoTheme.Typo.body)),
        Role(
            name: "Row sub", spec: "15 / 20 regular", use: "Second lines, trailing changes, stat labels",
            sample: .text("AC Capital · Mia · 4h left", MonacoTheme.Typo.subhead)),
        Role(
            name: "Home figure", spec: "32 / 36 semibold", use: "Home's net worth only",
            sample: .figure("$12,082", ".00", .home)),
        Role(
            name: "Money hero", spec: "44 / 48 semibold",
            use: "Keypad amount only", sample: .figure("$250", "", .hero)),
    ]

    private let extras: [(label: String, font: Font)] = [
        ("Subhead strong · 15 semibold", MonacoTheme.Typo.subheadStrong),
        ("Caption · 15 medium", MonacoTheme.Typo.caption),
        ("Caption strong · 15 semibold", MonacoTheme.Typo.captionStrong),
    ]

    private let rules: [String] = [
        "Nothing under 13 pt in the app (Caption only in widgets, Live Activities, the tab bar and share cards, where the system sets it).",
        "No caps except ticker tiles and system group headers. No SF Mono except the wallet address. Tabular figures everywhere money appears.",
        "Symbols: SF Symbols regular, 22 in rows, 20 in bar circles, 17 inline, 15 in count lines and chips.",
        "System surfaces keep the system sizes: status bar, inline nav title, tab bar, keyboard, text fields, menus, notifications, Live Activities, widgets, Lock Screen.",
    ]

    var body: some View {
        GalleryPage(title: "Type") {
            GalleryGroup(title: "Roles") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    ForEach(roles, id: \.name) { role in
                        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                            sample(role.sample)
                            Text("\(role.name) · \(role.spec)")
                                .font(MonacoTheme.Typo.caption)
                                .foregroundStyle(MonacoTheme.ink)
                            GalleryCaption(role.use)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .accessibilityElement(children: .combine)
                    }
                    ForEach(extras, id: \.label) { role in
                        GalleryCase(caption: role.label) {
                            Text("Closes in 2d")
                                .font(role.font)
                                .foregroundStyle(MonacoTheme.ink)
                        }
                    }
                }
            }
            GalleryGroup(title: "Rules") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    ForEach(rules, id: \.self) { rule in
                        Text(rule)
                            .font(MonacoTheme.Typo.body)
                            .foregroundStyle(MonacoTheme.secondaryText)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }
            GalleryAccessibilitySample()
        }
    }

    @ViewBuilder
    private func sample(_ sample: Sample) -> some View {
        switch sample {
        case .text(let text, let font):
            Text(text)
                .font(font)
                .foregroundStyle(MonacoTheme.ink)
        case .figure(let whole, let cents, let style):
            HStack(alignment: .firstTextBaseline, spacing: 0) {
                Text(whole).foregroundStyle(MonacoTheme.primaryText)
                Text(cents).foregroundStyle(MonacoTheme.secondaryText)
            }
            .moneyFont(style)
            .lineLimit(1)
            .minimumScaleFactor(style.minimumScaleFactor)
        case .percent(let basisPoints, let style):
            PercentText(basisPoints: basisPoints, style: style)
        }
    }
}

struct GalleryAccessibilitySample: View {
    var body: some View {
        GalleryGroup(title: "Accessibility 3") {
            GalleryCase(caption: "Roles and figures at .accessibility3") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    Text("Weekend investors")
                        .font(MonacoTheme.Typo.display)
                    Text("Needs your vote")
                        .font(MonacoTheme.Typo.headline)
                    Text("Fund, then propose the first buy.")
                        .font(MonacoTheme.Typo.body)
                        .foregroundStyle(MonacoTheme.bodyText)
                    MoneyText(decimalString: "1248.50", style: .hero)
                    Text("Closes in 2d")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                }
                .foregroundStyle(MonacoTheme.ink)
                .dynamicTypeSize(.accessibility3)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }
}
#endif
