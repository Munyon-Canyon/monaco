#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

struct GalleryCardsPage: View {
    var body: some View {
        GalleryPage(title: "Cards") {
            GalleryCoverCardSamples()
            GalleryBoardToolSamples()
            GalleryBoardRowSamples()
            GalleryBoardListSamples()
            GalleryBalanceSamples()
        }
    }
}

enum GalleryCabalCards {
    static func model(
        id: String, tint: MonacoTheme.CabalTint, votes: String? = nil, funded: Bool = true,
        pnl: CabalCoverCardModel.Figure? = nil, reason: String? = nil, withPicture: Bool = false
    ) -> CabalCoverCardModel {
        let gained = pnl?.direction != .loss
        return CabalCoverCardModel(
            id: id, name: "Weekend Warriors", memberCountText: "8 members", votesNeededText: votes,
            shareText: funded ? "$7,000.00" : nil, ownershipText: funded ? "35.00%" : nil, pnl: pnl,
            pnlUnavailableReason: reason,
            groupReturn: .init(
                periodText: "1M", percentText: gained ? "+4.12%" : "−4.12%", direction: gained ? .profit : .loss),
            tint: tint,
            pictureURL: withPicture ? GalleryFixtures.picture(tint, name: "gallery-card-\(id)")?.absoluteString : nil)
    }

    static let loss = CabalCoverCardModel.Figure(amountText: "−$300.00", percentText: "−4.11%", direction: .loss)
    static let gain = CabalCoverCardModel.Figure(amountText: "+$412.50", percentText: "+5.90%", direction: .profit)
}

struct GalleryCoverCardSamples: View {
    private let samples: [(caption: String, model: CabalCoverCardModel)] = [
        (
            "List, ready with vote needed",
            GalleryCabalCards.model(
                id: "gallery-card-1", tint: .pine, votes: "1 vote needed", pnl: GalleryCabalCards.loss)
        ),
        ("List, no action", GalleryCabalCards.model(id: "gallery-card-2", tint: .plum, pnl: GalleryCabalCards.loss)),
        (
            "List, no share",
            GalleryCabalCards.model(id: "gallery-card-3", tint: .ochre, funded: false)
        ),
        (
            "List, P&L unavailable",
            GalleryCabalCards.model(id: "gallery-card-4", tint: .moss, reason: "—")
        ),
        (
            "List, gain, with a picture",
            GalleryCabalCards.model(
                id: "gallery-card-5", tint: .indigo, pnl: GalleryCabalCards.gain, withPicture: true)
        ),
    ]

    var body: some View {
        GalleryGroup(title: "Cabal cover cards") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                ForEach(samples, id: \.caption) { sample in
                    GalleryCase(caption: sample.caption) {
                        CabalCoverCard(model: sample.model, layout: .list, onOpen: {}, onShare: {})
                    }
                }
                GalleryCase(caption: "Rail, three cards") {
                    CabalCoverRail {
                        ForEach(samples.prefix(3), id: \.caption) { sample in
                            CabalCoverCard(model: sample.model, layout: .rail, onOpen: {}, onShare: {})
                        }
                    }
                    .monacoBleed()
                }
                GalleryCase(caption: "Skeleton, list and rail") {
                    VStack(spacing: MonacoTheme.Space.sm) {
                        CabalCoverCardSkeleton(layout: .list)
                        CabalCoverCardSkeleton(layout: .rail)
                    }
                }
            }
        }
    }
}

struct GalleryBoardToolSamples: View {
    @State private var category = "Cabals"
    @State private var period = LeaderboardRange.oneMonth
    @State private var fixed = "Cabals"

    var body: some View {
        GalleryGroup(title: "Board tools") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Category select, period select, filter, rank change") {
                    BoardTools(
                        category: $category, categories: ["Cabals", "People"], categoryLabel: { $0 },
                        period: $period, periods: LeaderboardRange.allCases, periodLabel: { $0.label },
                        contextText: "Cabal P&L · 1M", onFilter: {}, onRankInfo: {}
                    )
                    .monacoBleed()
                }
                GalleryCase(caption: "One category, no rank change") {
                    BoardTools(
                        category: $fixed, categories: ["Cabals"], categoryLabel: { $0 }, period: $period,
                        periods: LeaderboardRange.allCases, periodLabel: { $0.label },
                        contextText: "Cabal P&L · 1M", onFilter: {}
                    )
                    .monacoBleed()
                }
            }
        }
    }
}

struct GalleryBoardRowSamples: View {
    private struct Sample: Identifiable {
        let caption: String
        let row: LeaderboardRowView
        let movement: Int?
        var figure = BoardRow<MonacoAvatar>.Figure.pnl

        var id: String { caption }
    }

    private var samples: [Sample] {
        [
            Sample(
                caption: "Rank 1 with crown, up 2", row: Self.person(1, pnl: 1_240_000_000, bps: 4_230), movement: 2),
            Sample(caption: "Rank 2, down 1", row: Self.person(2, pnl: 610_000_000, bps: 1_810), movement: -1),
            Sample(caption: "Rank 3, unchanged", row: Self.person(3, pnl: 120_000_000, bps: 240), movement: 0),
            Sample(caption: "Rank 4, no movement", row: Self.person(4, pnl: 20_000_000, bps: 40), movement: nil),
            Sample(caption: "Loss", row: Self.person(5, pnl: -310_000_000, bps: -730), movement: nil),
            Sample(
                caption: "Value figure for a cabal", row: Self.person(6, pnl: 90_000_000, bps: 120), movement: nil,
                figure: .value),
        ]
    }

    private static func person(_ rank: Int32, pnl: Int64, bps: Int64) -> LeaderboardRowView {
        LeaderboardRowView(
            Components.Schemas.LeaderboardRow(
                rank: rank,
                subject: .sample(id: "gallery-person-\(rank)", name: "Investor \(rank)", handle: "investor\(rank)"),
                valueMicros: 4_800_000_000, pnlMicros: pnl, returnBps: bps, flags: []))
    }

    var body: some View {
        GalleryGroup(title: "Rank rows") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                ForEach(samples) { sample in
                    GalleryCase(caption: sample.caption) {
                        BoardRow(row: sample.row, figure: sample.figure, isLast: true, movement: sample.movement) {
                            MonacoAvatar(photoURL: nil, displayName: sample.row.name, size: .rank, seed: sample.row.id)
                        }
                        .monacoBleed()
                    }
                }
            }
        }
    }
}

struct GalleryBoardListSamples: View {
    var body: some View {
        GalleryGroup(title: "Board list") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryBoard(caption: "Loading", transport: GalleryBoardTransport(behavior: .hang))
                GalleryBoard(caption: "Empty", transport: .page(rows: 0, hasMore: false))
                GalleryBoard(caption: "Error", transport: GalleryBoardTransport(behavior: .fail))
                GalleryBoard(
                    caption: "Load more, with the viewer pinned",
                    transport: .page(rows: 3, hasMore: true, viewerRank: 14))
                GalleryBoard(caption: "All shown", transport: .page(rows: 3, hasMore: false))
            }
        }
    }
}

struct GalleryBalanceSamples: View {
    var body: some View {
        GalleryGroup(title: "Home balance") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                GalleryCase(caption: "Zero") { HomeBalanceSummary(display: .amount(0)).monacoBleed() }
                GalleryCase(caption: "Small") { HomeBalanceSummary(display: .amount(1_990_000)).monacoBleed() }
                GalleryCase(caption: "Large") { HomeBalanceSummary(display: .amount(12_345_670_000)).monacoBleed() }
                GalleryCase(caption: "Pending and arriving") {
                    HomeBalanceSummary(
                        display: .amount(198_500_000), status: PlatformBalanceCard.pendingLine(micros: 50_000_000),
                        arrivingMicros: 4_500_000
                    )
                    .monacoBleed()
                }
                GalleryCase(caption: "Loading") { HomeBalanceSummary(display: .loading).monacoBleed() }
                GalleryCase(caption: "Unavailable") {
                    HomeBalanceSummary(display: .unavailable, retry: {}).monacoBleed()
                }
            }
        }
    }
}
#endif
