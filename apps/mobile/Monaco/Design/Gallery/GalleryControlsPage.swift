#if DEBUG
import MonacoCore
import SwiftUI

struct GalleryControlsPage: View {
    var body: some View {
        GalleryPage(title: "Controls") {
            GalleryBoardControlSamples()
            GalleryVoteSamples()
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

struct GalleryUsageRow<Content: View>: View {
    let title: String
    let note: String
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(title).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
                GalleryCaption(note)
            }
            content
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct GalleryBoardControlSamples: View {
    private static let tabs = ["Treasury", "Rules", "Members", "Voters"]
    private static let ranges = ["1H", "1D", "1W", "1M", "3M", "6M", "All"]
    private static let periods = ["Week", "Month", "All time"]
    private static let filters = ["All", "Watchlist", "Pre-IPO", "Held"]

    @State private var tab = "Treasury"
    @State private var range = "1M"
    @State private var period = "Month"
    @State private var filter = "All"

    var body: some View {
        GalleryGroup(title: "Controls, which control where") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                basics
                votes
                facts
                social
            }
        }
    }

    private var basics: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            GalleryUsageRow(title: "Underline tabs", note: "sibling sections of one screen · max 1 row") {
                MonacoUnderlineTabs(
                    Self.tabs, selection: $tab, count: { $0 == "Members" ? 8 : nil },
                    identifier: { "gallery-tab-\($0)" }
                ) { $0 }
                .monacoBleed()
            }
            GalleryUsageRow(
                title: "Chart ranges · periods",
                note: "plain labels, no underline, no pill; the board's Week · Month · All time too"
            ) {
                MonacoRangeChips(
                    ranges: Self.ranges, selection: range, identifierPrefix: "gallery-range", label: { $0 },
                    onSelect: { range = $0 })
                MonacoPeriodLabels(
                    title: "This month", dates: "Oct 1 – 31", periods: Self.periods, selection: $period,
                    identifierPrefix: "gallery-period-header", label: { $0 }
                )
                .monacoBleed()
            }
            GalleryUsageRow(title: "Chips", note: "filters, quick amounts, networks · max 4") {
                MonacoChipBar(
                    items: Self.filters, selected: filter, title: { $0 }, identifierPrefix: "gallery-filter",
                    select: { filter = $0 }
                )
                .monacoBleed()
            }
            GalleryUsageRow(title: "Buttons", note: "one prominent per screen") {
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
                HStack {
                    Button("Deposit") {}.buttonStyle(.monacoCompactProminent)
                    Spacer()
                    Button("See all") {}.buttonStyle(.monacoText)
                }
            }
        }
    }

    private var votes: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            GalleryUsageRow(
                title: "Two-ended tally · idle",
                note:
                    "theme 1: the bar fills Yes from the left and No from the right; neutral pair with the live counts"
            ) {
                VoteTallyBar(yes: 4, no: 1, eligible: 8)
                VoteHoldRow(
                    yes: VoteHoldButton(side: .yes, count: 4, phase: .idle),
                    no: VoteHoldButton(side: .no, count: 1, phase: .idle))
                VoteStatusLine(
                    result: nil, facts: ["5 of 8 voted", "6 replies", "ends in 4h"], identifierPrefix: "gallery-usage",
                    identifierSuffix: "idle")
            }
            GalleryUsageRow(
                title: "Pick → hold → committed",
                note: "0.6 s, F's ring and sweep are the progress; the count ticks on commit"
            ) {
                VoteHoldRow(
                    yes: VoteHoldButton(side: .yes, count: 4, phase: .picked),
                    no: VoteHoldButton(side: .no, count: 1, phase: .dimmed))
                VoteHoldRow(
                    yes: VoteHoldButton(side: .yes, count: 4, phase: .holding(progress: 0.4)),
                    no: VoteHoldButton(side: .no, count: 1, phase: .dimmed))
                VoteHoldRow(
                    yes: VoteHoldButton(side: .yes, count: 5, phase: .committed),
                    no: VoteHoldButton(side: .no, count: 1, phase: .dimmed))
            }
            GalleryUsageRow(
                title: "Closed · sides",
                note:
                    "the counts at the two ends; the names by side only when the cabal's Rules switch is on (decision 6)"
            ) {
                VoteTallyBar(yes: 5, no: 1, eligible: 8, winner: .yes)
                VoteTallyEnds(
                    yes: 5, no: 1, middle: "6 of 8 voted",
                    identifierPrefix: "gallery-usage", identifierSuffix: "closed")
                VoteStatusLine(
                    result: .passed, facts: ["filled at $219.40", "Oct 22"], identifierPrefix: "gallery-usage",
                    identifierSuffix: "passed")
                VoteSidesRow(
                    lines: [
                        .init(side: .yes, avatars: Self.people, text: "Mia, Christelle, Logan, Sam, you"),
                        .init(side: .no, avatars: [Self.people[1]], text: "Kai"),
                        .init(
                            side: nil, avatars: [Self.people[2], Self.people[3]], text: "Jordan and Priya did not vote"),
                    ], identifierPrefix: "gallery-usage")
            }
        }
    }

    private var facts: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            GalleryUsageRow(title: "Proposal stat row", note: "D's four facts under the hero (decision 10)") {
                StatGrid(items: [
                    StatItem(label: "Price", value: "$220.00", info: {}),
                    StatItem(label: "Shares", value: "≈ 1.82"),
                    StatItem(label: "Of treasury", value: "2%"),
                    StatItem(label: "Closes", value: "1:53 PM"),
                ])
            }
            GalleryUsageRow(
                title: "Remind me at close", note: "theme 8: 32 visible, 44 to tap; gone once you have voted"
            ) {
                RemindControl(isOn: false, onTitle: "Reminder set") {}
            }
            GalleryUsageRow(
                title: "Ranked row",
                note:
                    "decision 2: the rate over its record, turnout as the sub-line; under five resolved votes, no rank"
            ) {
                VStack(spacing: 0) {
                    RankedRow(
                        rank: .ranked(8, movement: 1), name: "Andres", isYou: true, subtitle: "Voted 11 of 11",
                        figure: "78%", detail: "7 of 9 right"
                    ) { avatar("Andres") }
                    RankedRow(
                        rank: .unranked, name: "Rosa", isYou: false, subtitle: "2 resolved · voted 3 of 4",
                        figure: "3 more", detail: "to rank"
                    ) { avatar("Rosa") }
                }
                .monacoBleed()
            }
        }
    }

    private var social: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            GalleryUsageRow(
                title: "Reactions",
                note:
                    "decision 5: any emoji as a chip with its count; the quick six and +; then the reply control, bubble and count, that opens the thread (10-27)"
            ) {
                HStack(spacing: MonacoTheme.Space.s) {
                    MonacoReactionChip(emoji: "💪", count: 3, mine: false) {}
                    MonacoReactionChip(emoji: "🙌", count: 1, mine: true) {}
                    MonacoReactionChip.add {}
                    MonacoReactionChip.reply(count: 6) {}
                }
                HStack(spacing: MonacoTheme.Space.sm) {
                    ForEach(["👍", "❤️", "😂", "🎉", "👀", "💪"], id: \.self) { Text($0).font(MonacoTheme.Typo.headline) }
                    Image(systemName: MonacoIcon.plus.symbol).foregroundStyle(MonacoTheme.secondaryText)
                }
                .padding(.horizontal, MonacoTheme.Space.m)
                .padding(.vertical, MonacoTheme.Space.sm)
                .background(MonacoTheme.surfaceSunken, in: Capsule())
            }
            GalleryUsageRow(title: "GIF", note: "decision 5: the composer's GIF button; a GIF reply is a bubble") {
                HStack(spacing: MonacoTheme.Space.sm) {
                    Image(systemName: MonacoIcon.plus.symbol).foregroundStyle(MonacoTheme.tint)
                    HStack {
                        Text("Say something").font(MonacoTheme.Typo.body).foregroundStyle(MonacoTheme.secondaryText)
                        Spacer()
                        Text(CommentsCopy.gifTitle)
                            .font(MonacoTheme.Typo.captionStrong)
                            .foregroundStyle(MonacoTheme.secondaryText)
                            .padding(.horizontal, MonacoTheme.Space.s)
                            .padding(.vertical, MonacoTheme.Space.xs)
                            .overlay(
                                RoundedRectangle(cornerRadius: MonacoTheme.Radius.pill, style: .continuous)
                                    .strokeBorder(MonacoTheme.secondaryText, lineWidth: 1))
                    }
                    .padding(.horizontal, MonacoTheme.Space.m)
                    .padding(.vertical, MonacoTheme.Space.sm)
                    .background(MonacoTheme.surfaceSunken, in: Capsule())
                }
            }
            GalleryUsageRow(title: "Crest", note: "theme 12: the cover cut square, 6 pt; people stay circles") {
                HStack(spacing: MonacoTheme.Space.sm) {
                    CabalMark(
                        groupId: GalleryFixtures.cabalNames[0].id,
                        name: GalleryFixtures.cabalNames[0].name, size: .header)
                    CabalMark(
                        groupId: GalleryFixtures.cabalNames[1].id,
                        name: GalleryFixtures.cabalNames[1].name, size: .row)
                    CabalMark(
                        groupId: GalleryFixtures.cabalNames[2].id,
                        name: GalleryFixtures.cabalNames[2].name, size: .small)
                    avatar("Mia")
                }
            }
            GalleryUsageRow(title: "Toast", note: "material capsule, 2.5 s, in the nav row over the title") {
                MonacoToastBanner(
                    message: "Voted Yes. 6th of 8.", isSuccess: true, action: MonacoToastAction(title: "Undo") {},
                    insetsGutter: false)
            }
        }
    }

    private static let people: [MonacoAvatar.Source] = ["Mia", "Christelle", "Logan", "Sam", "You"].map {
        MonacoAvatar.Source(id: $0, displayName: $0, photoURL: nil)
    }

    private func avatar(_ name: String) -> some View {
        MonacoAvatar(photoURL: nil, displayName: name, size: 40, seed: "gallery-usage-\(name)")
    }
}

struct GalleryVoteSamples: View {
    @State private var live = VoteSelection.none

    var body: some View {
        GalleryGroup(title: "Vote") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                ForEach([ColorScheme.dark, .light], id: \.self) { scheme in
                    GalleryVoteStates()
                        .padding(MonacoTheme.Space.m)
                        .background(MonacoTheme.canvas)
                        .environment(\.colorScheme, scheme)
                }
                GalleryCase(caption: "Live pair: press and hold for 0.6 s, lift early to cancel") {
                    VotePair(
                        yes: 4, no: 1, selection: $live, isEnabled: true, identifierPrefix: "gallery",
                        onCommit: { _ in
                            Task {
                                try? await Task.sleep(for: .milliseconds(500))
                                if case .pending(let side) = live { live = .committed(side) }
                            }
                        })
                    Button("Reset") { live = .none }.buttonStyle(.monacoText)
                }
            }
        }
    }
}

private struct GalleryVoteStates: View {
    private static let people: [MonacoAvatar.Source] = ["Mia", "Christelle", "Logan", "Sam", "You"].map {
        MonacoAvatar.Source(id: $0, displayName: $0, photoURL: nil)
    }

    private func row(_ yes: VoteHoldPhase, _ no: VoteHoldPhase, counts: (Int, Int) = (4, 1)) -> some View {
        VoteHoldRow(
            yes: VoteHoldButton(side: .yes, count: counts.0, phase: yes),
            no: VoteHoldButton(side: .no, count: counts.1, phase: no))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            GalleryCase(caption: "Two-ended tally, idle") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    VoteTallyBar(yes: 4, no: 1, eligible: 8)
                    row(.idle, .idle)
                    VoteStatusLine(
                        result: nil, facts: ["5 of 8 voted", "6 replies", "ends in 4h"], identifierPrefix: "gallery",
                        identifierSuffix: "idle")
                }
            }
            GalleryCase(caption: "Picked, holding at 40 %, committed") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    row(.picked, .dimmed)
                    row(.holding(progress: 0.4), .dimmed)
                    Text("Keep holding · lift to cancel")
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                    row(.committed, .dimmed, counts: (5, 1))
                    VoteStatusLine(
                        result: .executing, facts: ["Passed 5–1", "6 of 8 voted", "buying 1.82 sh"],
                        identifierPrefix: "gallery", identifierSuffix: "executing")
                }
            }
            GalleryCase(caption: "Closed, passed: ends, winner in label colour") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    VoteTallyBar(yes: 4, no: 1, eligible: 6, winner: .yes)
                    VoteTallyEnds(
                        yes: 4, no: 1, middle: "5 of 6 voted", identifierPrefix: "gallery", identifierSuffix: "closed")
                    VoteStatusLine(
                        result: .passed, facts: ["Passed 4–1", "filled at $716.52"], identifierPrefix: "gallery",
                        identifierSuffix: "passed")
                }
            }
            GalleryCase(caption: "Closed, failed") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    VoteTallyBar(yes: 2, no: 2, eligible: 6)
                    VoteStatusLine(
                        result: .failed, facts: ["Failed 2–2", "4 of 6 voted"], identifierPrefix: "gallery",
                        identifierSuffix: "failed")
                }
            }
            GalleryCase(caption: "Open card a non-member sees: no pair, no Remind") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    VoteTallyBar(yes: 3, no: 0, eligible: 7)
                    VoteTallyEnds(
                        yes: 3, no: 0, middle: "Pilates members vote", identifierPrefix: "gallery",
                        identifierSuffix: "public")
                    VoteStatusLine(
                        result: nil, facts: ["3 of 7 voted", "2 replies", "ends in 23h"], identifierPrefix: "gallery",
                        identifierSuffix: "public")
                }
            }
            GalleryCase(caption: "Sides row") {
                VoteSidesRow(
                    lines: [
                        .init(side: .yes, avatars: Self.people, text: "Mia, Christelle, Logan, Sam, you"),
                        .init(side: .no, avatars: [Self.people[1]], text: "Kai"),
                        .init(
                            side: nil, avatars: [Self.people[2], Self.people[3]], text: "Jordan and Priya did not vote"),
                    ], identifierPrefix: "gallery")
            }
        }
    }
}
#endif
