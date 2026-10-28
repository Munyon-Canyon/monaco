#if DEBUG
import MonacoCore
import SwiftUI

struct GalleryControlsPage: View {
    var body: some View {
        GalleryPage(title: "Controls") {
            GalleryBoardControlSamples()
            GalleryButtonSamples()
            GalleryCompactSamples()
            GalleryDockSamples()
            GallerySelectionSamples()
            GalleryFieldSamples()
            GalleryAmountSamples()
        }
    }
}

struct GalleryButtonRow<Style: ButtonStyle>: View {
    let name: String
    let style: Style
    let idle: String
    let working: String
    var tint = MonacoTheme.primaryButtonLabel

    var body: some View {
        GalleryCase(caption: "\(name): enabled, disabled, submitting") {
            VStack(spacing: MonacoTheme.Space.s) {
                HStack(spacing: MonacoTheme.Space.s) {
                    Button(idle) {}.buttonStyle(style)
                    Button(idle) {}.buttonStyle(style).disabled(true)
                }
                Button {
                } label: {
                    SubmitLabel(isWorking: true, idle: idle, working: working, tint: tint)
                }
                .buttonStyle(style)
                .disabled(true)
            }
        }
    }
}

struct GalleryButtonSamples: View {
    var body: some View {
        GalleryGroup(title: "Buttons") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryButtonRow(name: "Primary", style: .monacoPrimary, idle: "Add money", working: "Adding…")
                GalleryButtonRow(
                    name: "Secondary", style: .monacoSecondary, idle: "Cash out", working: "Cashing out…",
                    tint: MonacoTheme.secondaryButtonLabel)
                GalleryButtonRow(
                    name: "Outline", style: .monacoOutline, idle: "Vote no", working: "Voting…",
                    tint: MonacoTheme.ink)
                GalleryButtonRow(
                    name: "Danger", style: .monacoDestructive, idle: "Leave cabal", working: "Leaving…",
                    tint: MonacoTheme.onDestructive)
            }
        }
    }
}

struct GalleryCompactSamples: View {
    var body: some View {
        GalleryGroup(title: "Small, links and tiles") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Small (32 in a 44 hit box): quiet, quiet disabled, prominent") {
                    GalleryGrid(minimum: 110) {
                        Button("Review") {}.buttonStyle(.monacoCompact)
                        Button("Status") {}.buttonStyle(.monacoCompact).disabled(true)
                        Button("Deposit") {}.buttonStyle(.monacoCompactProminent)
                    }
                }
                GalleryCase(caption: "Text link, with chevron; load more") {
                    VStack(alignment: .leading, spacing: 0) {
                        Button("See all") {}.buttonStyle(.monacoText)
                        Button("See all") {}.buttonStyle(.monacoText(chevron: true))
                        Button("Load more") {}.buttonStyle(.monacoLoadMore)
                    }
                }
                GalleryCase(caption: "Action tiles: enabled, enabled, disabled") {
                    GalleryGrid(minimum: 80) {
                        CircleAction("Deposit", systemImage: MonacoIcon.deposit.symbol) {}
                        CircleAction("Propose", systemImage: MonacoIcon.propose.symbol) {}
                        CircleAction("Cash out", systemImage: MonacoIcon.cashOut.symbol) {}.disabled(true)
                    }
                }
            }
        }
    }
}

struct GalleryDockSamples: View {
    var body: some View {
        GalleryGroup(title: "Dock") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "BottomCTA, one button") {
                    BottomCTA { Button("Review") {}.buttonStyle(.monacoPrimary) }.monacoBleed()
                }
                GalleryCase(caption: "BottomCTA, outline and primary") {
                    BottomCTA {
                        Button("Vote no") {}.buttonStyle(.monacoOutline)
                        Button("Vote yes") {}.buttonStyle(.monacoPrimary)
                    }
                    .monacoBleed()
                }
                GalleryCase(caption: "BottomCTA, with a caption") {
                    BottomCTA(caption: "Voting closes in 2 days") {
                        Button("Propose a buy") {}.buttonStyle(.monacoPrimary)
                    }
                    .monacoBleed()
                }
            }
        }
    }
}

struct GallerySelectionSamples: View {
    @State private var two = "Open"
    @State private var three = "All"
    @State private var period = LeaderboardRange.oneWeek
    @State private var sort = "Top"
    @State private var isOn = true

    var body: some View {
        GalleryGroup(title: "Selection") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Segmented, two options") {
                    MonacoSegmented(["Open", "Closed"], selection: $two) { $0 }
                }
                GalleryCase(caption: "Segmented, three options") {
                    MonacoSegmented(["All", "Mine", "Voted"], selection: $three) { $0 }
                }
                GalleryCase(caption: "Periods") {
                    MonacoRangeChips(
                        ranges: [LeaderboardRange.oneDay, .oneWeek, .oneMonth, .all], selection: period,
                        identifierPrefix: "gallery-periods", onSelect: { period = $0 })
                }
                chips
                GalleryCase(caption: "Select menu") {
                    MonacoMenuSelect(title: "Sort", selection: $sort, options: ["Top", "New"]) { $0 }
                }
                GalleryCase(caption: "Switch, on and off; selection tick, on and off") {
                    HStack(spacing: MonacoTheme.Space.l) {
                        Toggle("Alerts", isOn: $isOn).labelsHidden().monacoSwitch()
                        Toggle("Alerts", isOn: .constant(false)).labelsHidden().monacoSwitch()
                        MonacoSelectionTick(isSelected: true)
                        MonacoSelectionTick(isSelected: false)
                    }
                }
            }
        }
    }

    private var chips: some View {
        GalleryCase(caption: "Chips, rest and on") {
            GalleryGrid(minimum: 90) {
                Text("$25").monacoChipLabel(isSelected: false)
                Text("$50").monacoChipLabel(isSelected: true)
                Text("Max").monacoChipLabel(isSelected: false)
            }
        }
    }
}

struct GalleryBoardControlSamples: View {
    private static let tabs = ["Treasury", "Rules", "Members", "Voters"]
    private static let ranges = ["1H", "1D", "1W", "1M", "3M", "6M", "All"]
    private static let periods = ["Week", "Month", "All time"]
    private static let filters = ["All", "Watchlist", "Pre-IPO", "Held"]
    private static let networks = ["Solana", "Base", "Arbitrum", "Ethereum"]

    @State private var tab = "Treasury"
    @State private var range = "1M"
    @State private var period = "Month"
    @State private var filter = "All"
    @State private var network = "Solana"

    var body: some View {
        GalleryGroup(title: "Board controls") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Underline tabs") {
                    MonacoUnderlineTabs(
                        Self.tabs, selection: $tab, count: { $0 == "Members" ? 8 : nil },
                        identifier: { "gallery-tab-\($0)" }
                    ) { $0 }
                    .monacoBleed()
                }
                GalleryCase(caption: "Chart ranges, plain labels") {
                    MonacoRangeChips(
                        ranges: Self.ranges, selection: range, identifierPrefix: "gallery-range", label: { $0 },
                        onSelect: { range = $0 })
                }
                GalleryCase(caption: "Period header") {
                    MonacoPeriodLabels(
                        title: "This month", dates: "Oct 1 – 31", periods: Self.periods, selection: $period,
                        identifierPrefix: "gallery-period-header", label: { $0 }
                    )
                    .monacoBleed()
                }
                GalleryCase(caption: "Filter chips") {
                    MonacoChipBar(
                        items: Self.filters, selected: filter, title: { $0 }, identifierPrefix: "gallery-filter",
                        select: { filter = $0 }
                    )
                    .monacoBleed()
                }
                GalleryCase(caption: "Network chips, the selected one checked") {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: MonacoTheme.Space.s) {
                            ForEach(Self.networks, id: \.self) { name in
                                Button {
                                    network = name
                                } label: {
                                    Text(name).monacoChipLabel(isSelected: name == network, leadingSymbol: .check)
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                }
                GalleryCase(caption: "Prominent and quiet, a row of two") {
                    HStack(spacing: MonacoTheme.Space.s) {
                        Button {
                        } label: {
                            Label("Fund", systemImage: MonacoIcon.plus.symbol)
                        }
                        .buttonStyle(.monacoPrimary)
                        Button {
                        } label: {
                            Label("Propose", systemImage: MonacoIcon.propose.symbol)
                        }
                        .buttonStyle(.monacoSecondary)
                    }
                }
                GalleryCase(caption: "Small prominent Deposit; text link") {
                    HStack {
                        Button("Deposit") {}.buttonStyle(.monacoCompactProminent)
                        Spacer()
                        Button("See all") {}.buttonStyle(.monacoText)
                    }
                }
            }
        }
    }
}
#endif
