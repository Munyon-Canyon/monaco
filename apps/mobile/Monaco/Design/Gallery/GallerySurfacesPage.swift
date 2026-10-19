#if DEBUG
import SwiftUI

struct GallerySurfacesPage: View {
    var body: some View {
        GalleryPage(title: "Surfaces") {
            GalleryCardSamples()
            GalleryRowSamples()
            GalleryHeaderSamples()
            GalleryNoticeSamples()
            GalleryBadgeSamples()
            GalleryEmptySamples()
            GalleryOutcomeSamples()
            GallerySkeletonSamples()
        }
    }
}

struct GalleryCardSamples: View {
    @State private var closed = false
    @State private var open = true

    private let address = "4Nd1mYQzvF3pTk8Lw2RbXc9HsJeUq6VaGt5DyKoZ1MnB"

    var body: some View {
        GalleryGroup(title: "Cards") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "MonacoCard, standard") {
                    MonacoCard { cardText("Your share is $311.50") }
                }
                GalleryCase(caption: "MonacoCard, tight") {
                    MonacoCard(style: .tight) { cardText("Closes in 2 days") }
                }
                GalleryCase(caption: "MonacoCard, inset with key rows and a receipt line") {
                    MonacoCard(style: .inset) {
                        MonacoKeyRows {
                            ReceiptLine(label: "Amount", value: .data("$400.00"))
                            ReceiptLine(label: "Status", value: .words("Completed"))
                            ReceiptLine(label: "Sent to", value: .address(address))
                        }
                    }
                }
                GalleryCase(caption: "Disclosure, closed") {
                    DisclosureGroup("How is this calculated?", isExpanded: $closed) {
                        cardText("Your share is your deposits over the pot.")
                    }
                    .disclosureGroupStyle(.monaco)
                }
                GalleryCase(caption: "Disclosure, open") {
                    DisclosureGroup("How is this calculated?", isExpanded: $open) {
                        cardText("Your share is your deposits over the pot.")
                    }
                    .disclosureGroupStyle(.monaco)
                }
            }
        }
    }

    private func cardText(_ text: String) -> some View {
        Text(text)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.ink)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct GalleryRowSamples: View {
    var body: some View {
        GalleryGroup(title: "Rows") {
            GalleryCase(caption: "MonacoRow: glyph tile, avatar, stock, with badge, with chevron") {
                MonacoGroupedList {
                    MonacoRow(title: "Deposit", subtitle: "Completed") {
                        SunkenGlyphMark(icon: .deposit)
                    } trailing: {
                        MoneyText(decimalString: "50.00", style: .row)
                    }
                    MonacoRow(title: "Maya Okafor", subtitle: "@maya") {
                        MonacoAvatar(photoURL: nil, displayName: "Maya Okafor", size: .rank)
                    }
                    MonacoRow(title: "NVIDIA", subtitle: "NVDA · 30.0000 units") {
                        StockMark(symbol: "NVDAx", size: .row)
                    } trailing: {
                        MoneyText(decimalString: "412.80", style: .row)
                        PnLText(dollarPnl: "+12.10", style: .caption)
                    }
                    MonacoRow(title: "Buy Apple", subtitle: "$400.00 from the pot") {
                        StockMark(symbol: "AAPLx", size: .row)
                    } trailing: {
                        StatusBadge("Needs your vote", tone: .attention)
                    }
                    MonacoRow(title: "Weekend investors", subtitle: "8 members", chevron: true, isLast: true) {
                        CabalMark(groupId: GalleryFixtures.cabalNames[0].id, name: GalleryFixtures.cabalNames[0].name)
                    }
                }
            }
        }
    }
}

struct GalleryHeaderSamples: View {
    var body: some View {
        GalleryGroup(title: "Section headers") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                GalleryCase(caption: "Plain") { MonacoSectionHeader("Holdings") }
                GalleryCase(caption: "With a count") { MonacoSectionHeader("Needs your vote", count: 2) }
                GalleryCase(caption: "With a link") { MonacoSectionHeader("Your cabals", trailing: "See all") {} }
            }
        }
    }
}
#endif
