#if DEBUG
import SwiftUI

struct GalleryNoticeSamples: View {
    var body: some View {
        GalleryGroup(title: "Notices") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                GalleryCase(caption: "Info, message only") {
                    MonacoNotice(.info, message: "Deposits arrive within a minute.")
                }
                GalleryCase(caption: "Info, title and action") {
                    MonacoNotice(
                        .info, title: "Opens soon", message: "Markets arrive with the next update.",
                        action: ("Remind me", {}))
                }
                GalleryCase(caption: "Warn, message only") {
                    MonacoNotice(.warn, message: "Prices are a few minutes behind.")
                }
                GalleryCase(caption: "Warn, title and action") {
                    MonacoNotice(
                        .warn, title: "Prices delayed", message: "We will refresh them shortly.",
                        action: ("Refresh", {}))
                }
                GalleryCase(caption: "Error, message only") {
                    MonacoNotice(.error, message: "More than the pot has.")
                }
                GalleryCase(caption: "Error, title and action") {
                    MonacoNotice(
                        .error, title: "Couldn't send", message: "Check your connection and try again.",
                        action: ("Try again", {}))
                }
                GalleryCase(caption: "Quiet empty line") {
                    MonacoQuietEmpty("No proposals yet", systemImage: MonacoIcon.propose.symbol)
                }
                GalleryCase(caption: "Opens soon") {
                    MonacoOpensSoon("Markets")
                }
            }
        }
    }
}

struct GalleryBadgeSamples: View {
    private let badges: [(name: String, text: String, tone: StatusBadge.Tone)] = [
        ("positive", "Voted yes", .positive), ("negative", "Not approved", .negative),
        ("warn", "Executing", .warn), ("neutral", "Not voted", .neutral),
        ("attention", "Needs your vote", .attention), ("discussion", "Discussion", .discussion),
    ]

    private let pills: [(name: String, text: String, tone: PnLPill.Tone)] = [
        ("gain", "+8.2% · 1M", .gain), ("loss", "−3.1% · 1M", .loss), ("muted", "P&L private", .muted),
    ]

    var body: some View {
        GalleryGroup(title: "Badges and pills") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "StatusBadge, six tones") {
                    GalleryGrid(minimum: 150) {
                        ForEach(badges, id: \.name) { badge in
                            GalleryCase(caption: badge.name) { StatusBadge(badge.text, tone: badge.tone) }
                        }
                    }
                }
                GalleryCase(caption: "PnLPill, gain, loss and muted") {
                    GalleryGrid(minimum: 120) {
                        ForEach(pills, id: \.name) { pill in
                            GalleryCase(caption: pill.name) { PnLPill(text: pill.text, tone: pill.tone) }
                        }
                    }
                }
                GalleryCase(caption: "EventPill, every kind") {
                    GalleryGrid(minimum: 120) {
                        ForEach(EventPill.Kind.allCases, id: \.self) { kind in
                            GalleryCase(caption: String(describing: kind)) { EventPill(kind: kind) }
                        }
                    }
                }
            }
        }
    }
}
#endif
