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
            GalleryRankedRowSamples()
            GalleryStatRowSamples()
            GalleryRemindSamples()
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
                id: "gallery-card-1", tint: .blue, votes: "1 vote needed", pnl: GalleryCabalCards.loss)
        ),
        ("List, no action", GalleryCabalCards.model(id: "gallery-card-2", tint: .purple, pnl: GalleryCabalCards.loss)),
        (
            "List, no share",
            GalleryCabalCards.model(id: "gallery-card-3", tint: .orange, funded: false)
        ),
        (
            "List, P&L unavailable",
            GalleryCabalCards.model(id: "gallery-card-4", tint: .teal, reason: "—")
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

struct GalleryRankedRowSamples: View {
    private let cabal = GalleryFixtures.cabalNames[0]

    private func avatar(_ name: String) -> some View {
        MonacoAvatar(photoURL: nil, displayName: name, size: 40, seed: "gallery-ranked-\(name)")
    }

    var body: some View {
        GalleryGroup(title: "Ranked rows") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                GalleryCase(caption: "Ranked and up, down, same, new") {
                    VStack(spacing: 0) {
                        RankedRow(
                            rank: .ranked(7, movement: 1), name: "Kai", isYou: false, subtitle: "Voted 14 of 14",
                            figure: "79%", detail: "11 of 14 right"
                        ) { avatar("Kai") }
                        RankedRow(
                            rank: .ranked(9, movement: -1), name: "Rosa", isYou: false, subtitle: "Voted 8 of 9",
                            figure: "71%", detail: "5 of 7 right"
                        ) { avatar("Rosa") }
                        RankedRow(
                            rank: .ranked(10, movement: 0), name: "Mo", isYou: false, subtitle: "Voted 9 of 11",
                            figure: "67%", detail: "6 of 9 right"
                        ) { avatar("Mo") }
                        RankedRow(
                            rank: .ranked(11, movement: nil), name: "Lena", isYou: false, subtitle: "Voted 5 of 5",
                            figure: "60%", detail: "3 of 5 right"
                        ) { avatar("Lena") }
                    }
                    .monacoBleed()
                }
                GalleryCase(caption: "Your row on the surface band") {
                    RankedRow(
                        rank: .ranked(8, movement: 1), name: "Andres", isYou: true, subtitle: "Voted 11 of 11",
                        figure: "78%", detail: "7 of 9 right"
                    ) { avatar("Andres") }
                    .monacoBleed()
                }
                GalleryCase(caption: "Unranked") {
                    VStack(spacing: 0) {
                        RankedRow(
                            rank: .unranked, name: "Rosa", isYou: false, subtitle: "2 resolved · voted 3 of 4",
                            figure: "3 more", detail: "to rank"
                        ) { avatar("Rosa") }
                        RankedRow(
                            rank: .unranked, name: "Andres", isYou: true, subtitle: "3 resolved · voted 3 of 3",
                            figure: "2 more", detail: "to rank"
                        ) { avatar("Andres") }
                    }
                    .monacoBleed()
                }
                GalleryCase(caption: "Crest-led cabal row") {
                    RankedRow(
                        rank: .ranked(3, movement: 2), name: cabal.name, isYou: false, subtitle: "Voted 12 of 12",
                        figure: "82%", detail: "9 of 11 right"
                    ) { CabalMark(groupId: cabal.id, name: cabal.name, size: .row) }
                    .monacoBleed()
                }
            }
        }
    }
}

struct GalleryStatRowSamples: View {
    var body: some View {
        GalleryGroup(title: "Stat rows") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Open proposal") {
                    StatGrid(items: [
                        StatItem(label: "Price", value: "$220.00", info: {}),
                        StatItem(label: "Shares", value: "≈ 1.82"),
                        StatItem(label: "Of treasury", value: "2%"),
                        StatItem(label: "Closes", value: "1:53 PM"),
                    ])
                }
                GalleryCase(caption: "Closed proposal, with a gain") {
                    StatGrid(items: [
                        StatItem(label: "Fill", value: "$218.40"),
                        StatItem(label: "Shares", value: "1.83"),
                        StatItem(label: "Now", value: "$220.80"),
                        StatItem(label: "Since", value: "+1.1%", tone: .gain),
                    ])
                }
                GalleryCase(caption: "Closed proposal, with a loss") {
                    StatGrid(items: [
                        StatItem(label: "Fill", value: "$218.40"),
                        StatItem(label: "Shares", value: "1.83"),
                        StatItem(label: "Now", value: "$214.00"),
                        StatItem(label: "Since", value: "−2.0%", tone: .loss),
                    ])
                }
                GalleryCase(caption: "Cabal header") {
                    StatGrid(items: [
                        StatItem(label: "Your share", value: "$311.50"),
                        StatItem(label: "Of treasury", value: "35%"),
                        StatItem(label: "Open votes", value: "1"),
                        StatItem(label: "Bot", value: "Off"),
                    ])
                }
                GalleryCase(caption: "Review sheet") {
                    StatGrid(items: [
                        StatItem(label: "Shares", value: "≈ 1.82"),
                        StatItem(label: "Price", value: "$220.00"),
                        StatItem(label: "Of treasury", value: "2%"),
                        StatItem(label: "To pass", value: "3 of 5"),
                    ])
                }
            }
        }
    }
}

struct GalleryRemindSamples: View {
    var body: some View {
        GalleryGroup(title: "Remind me at close") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                GalleryCase(caption: "Off") { RemindControl(isOn: false, onTitle: "Reminder at 12:53 PM") {} }
                GalleryCase(caption: "On") { RemindControl(isOn: true, onTitle: "Reminder at 12:53 PM") {} }
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
