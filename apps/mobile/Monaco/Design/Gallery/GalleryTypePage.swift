#if DEBUG
import SwiftUI

struct GalleryTypePage: View {
    private let roles: [(label: String, font: Font)] = [
        ("Typo.display · 30 bold", MonacoTheme.Typo.display),
        ("Typo.title · 28 semibold", MonacoTheme.Typo.title),
        ("Typo.section · 22 semibold", MonacoTheme.Typo.section),
        ("Typo.headline · 18 semibold", MonacoTheme.Typo.headline),
        ("Typo.body · 18 regular", MonacoTheme.Typo.body),
        ("Typo.subhead · 17 regular", MonacoTheme.Typo.subhead),
        ("Typo.subheadStrong · 17 semibold", MonacoTheme.Typo.subheadStrong),
        ("Typo.caption · 15 medium", MonacoTheme.Typo.caption),
        ("Typo.captionStrong · 15 semibold", MonacoTheme.Typo.captionStrong),
    ]

    private let figures: [(label: String, style: MoneyStyle)] = [
        ("moneyFont(.hero) · 44 bold", .hero),
        ("moneyFont(.large) · 35 bold", .large),
        ("moneyFont(.card) · 22 bold", .card),
        ("moneyFont(.row) · 18 semibold", .row),
        ("moneyFont(.caption) · 15 semibold", .caption),
    ]

    var body: some View {
        GalleryPage(title: "Type") {
            GalleryGroup(title: "Roles") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    ForEach(roles, id: \.label) { role in
                        GalleryCase(caption: role.label) {
                            Text("Weekend investors")
                                .font(role.font)
                                .foregroundStyle(MonacoTheme.ink)
                        }
                    }
                }
            }
            GalleryGroup(title: "Figures") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    ForEach(figures, id: \.label) { figure in
                        GalleryCase(caption: figure.label) {
                            MoneyText(decimalString: "1248.50", style: figure.style)
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
