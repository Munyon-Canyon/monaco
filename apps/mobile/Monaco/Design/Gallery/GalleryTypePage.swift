#if DEBUG
import SwiftUI

struct GalleryTypePage: View {
    private enum Sample {
        case text(String, Font)
        case money(String, MoneyStyle)
        case percent(Int64, MoneyStyle)
    }

    private let roles: [(label: String, sample: Sample)] = [
        ("Typo.display · 28 bold", .text("Markets", MonacoTheme.Typo.display)),
        ("Typo.title · 28 bold", .text("You're in.", MonacoTheme.Typo.title)),
        ("Typo.title2 · 22 bold", .text("AC Capital Management", MonacoTheme.Typo.title2)),
        ("moneyFont(.display) · 28 heavy", .money("20000.00", .display)),
        ("Typo.section · 17 semibold", .text("Your cabals", MonacoTheme.Typo.section)),
        ("Typo.button · 16 semibold", .text("Propose buy", MonacoTheme.Typo.button)),
        ("moneyFont(.card) · 20 semibold", .percent(790, .card)),
        ("Typo.rank · 15 semibold", .text("4", MonacoTheme.Typo.rank)),
        ("Typo.headline · 15 semibold", .text("Buy NVDA · $400", MonacoTheme.Typo.headline)),
        ("Typo.body · 15 regular", .text("Small add to the position.", MonacoTheme.Typo.body)),
        ("Typo.subhead · 15 regular", .text("AC Capital · Mia · 4h left", MonacoTheme.Typo.subhead)),
        ("moneyFont(.home) · 32 semibold", .money("12082.00", .home)),
        ("moneyFont(.hero) · 44 semibold", .money("250.00", .hero)),
    ]

    private let extras: [(label: String, font: Font)] = [
        ("Typo.subheadStrong · 15 semibold", MonacoTheme.Typo.subheadStrong),
        ("Typo.caption · 15 medium", MonacoTheme.Typo.caption),
        ("Typo.captionStrong · 15 semibold", MonacoTheme.Typo.captionStrong),
    ]

    var body: some View {
        GalleryPage(title: "Type") {
            GalleryGroup(title: "Roles") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    ForEach(roles, id: \.label) { role in
                        GalleryCase(caption: role.label) {
                            switch role.sample {
                            case .text(let text, let font):
                                Text(text)
                                    .font(font)
                                    .foregroundStyle(MonacoTheme.ink)
                            case .money(let amount, let style):
                                MoneyText(decimalString: amount, style: style)
                            case .percent(let basisPoints, let style):
                                PercentText(basisPoints: basisPoints, style: style)
                            }
                        }
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
            GalleryAccessibilitySample()
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
