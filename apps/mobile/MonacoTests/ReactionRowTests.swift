import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ReactionRowTests {
    private typealias Reaction = Components.Schemas.Reaction
    private typealias Row = ReactionRow<EmptyView>

    private func reaction(_ emoji: String, _ count: Int32, mine: Bool = false) -> Reaction {
        Reaction(emoji: emoji, count: count, mine: mine)
    }

    @Test func chipsAreTheTopThreeByCount() {
        let chips = Row.chips(for: [reaction("👀", 2), reaction("💪", 3), reaction("🎉", 5)])

        #expect(chips.map(\.id) == ["🎉", "💪", "👀"])
    }

    @Test func moreThanThreeEmojiFoldTheRestIntoAPlusN() {
        let chips = Row.chips(for: [reaction("🎉", 5), reaction("💪", 3), reaction("👀", 2), reaction("👏", 1)])

        #expect(chips == [.emoji(reaction("🎉", 5)), .emoji(reaction("💪", 3)), .more(3)])
    }

    @Test func aTieKeepsTheServersOrder() {
        let chips = Row.chips(for: [reaction("💪", 2), reaction("👀", 2)])

        #expect(chips.map(\.id) == ["💪", "👀"])
    }

    @Test func aNarrowRowGivesUpChipsBeforeItWraps() {
        #expect(Row.limits(for: 5) == [3, 2, 1])
        #expect(Row.limits(for: 2) == [2, 1])
        #expect(Row.limits(for: 0) == [1])
        #expect(Row.chips(for: [reaction("🎉", 5), reaction("💪", 3)], limit: 1) == [.more(8)])
    }

    @Test func mineIsTheChipsStylingFlag() {
        let mine = MonacoReactionChip(emoji: "🎉", count: 5, mine: true) {}
        let other = MonacoReactionChip(emoji: "👏", count: 2, mine: false) {}

        #expect(mine.isMine)
        #expect(!other.isMine)
        #expect(mine.spokenLabel == "party popper, 5 reactions, yours")
        #expect(other.spokenLabel == "clapping hands, 2 reactions")
        #expect(MonacoReactionChip(emoji: "👀", count: 1, mine: false) {}.spokenLabel == "eyes, 1 reaction")
    }

    @Test func theAddAndReplyChipsSayWhatTheyDo() {
        #expect(MonacoReactionChip.add {}.spokenLabel == "Add a reaction")
        #expect(MonacoReactionChip.reply(count: 0) {}.spokenLabel == "Reply")
        #expect(MonacoReactionChip.reply(count: 6) {}.spokenLabel == "6 replies")
    }

    @Test func showsAddFalseHidesThePlus() throws {
        let with = Row(
            target: .feedItem("a"), reactions: [reaction("🎉", 1)], onToggle: { _ in }, trailing: { EmptyView() })
        let without = Row(
            target: .feedItem("a"), reactions: [reaction("🎉", 1)], showsAdd: false, onToggle: { _ in },
            trailing: { EmptyView() })

        let shown = try identifiers(in: with)
        #expect(shown.contains("reaction-add-feed-a"))
        #expect(shown.contains("reaction-chip-feed-a-1F389"))
        #expect(try !identifiers(in: without).contains("reaction-add-feed-a"))
    }

    @Test func aChipIsFortyFourWideAndTheRowStaysOneLineAtTheLargestTextSize() {
        let row = ReactionRow<MonacoReactionChip>(
            target: .feedItem("a"),
            reactions: [reaction("🎉", 5), reaction("💪", 3), reaction("👀", 2), reaction("👏", 1)],
            onToggle: { _ in }
        ) { MonacoReactionChip.reply(count: 12) {} }
        .environment(\.dynamicTypeSize, .accessibility5)
        let size = UIHostingController(rootView: row).sizeThatFits(in: CGSize(width: 390, height: 2_000))

        #expect(size.height <= 120)
        let chip = UIHostingController(rootView: MonacoReactionChip.add {})
            .sizeThatFits(in: CGSize(width: 390, height: 2_000))
        #expect(chip.width >= MonacoTheme.ReactionChipMetrics.tapSize)
        #expect(chip.height >= MonacoTheme.ReactionChipMetrics.tapSize)
    }

    @Test func everyIconAddedHereResolvesToASystemSymbol() {
        let added: [MonacoIcon] = [
            .smiley, .hand, .leaf, .flag, .replyArrow, .plusCircle, .report, .thesis, .pollCheck,
        ]
        for icon in added {
            #expect(UIImage(systemName: icon.symbol) != nil, "\(icon) → \(icon.symbol)")
        }
    }

    private func identifiers(in view: some View) throws -> Set<String> {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = UIWindow(frame: CGRect(x: 0, y: 0, width: 390, height: 300))
        window.rootViewController = UIHostingController(rootView: view)
        window.makeKeyAndVisible()
        defer { TestWindow.close(window) }
        window.layoutIfNeeded()
        RunLoop.main.run(until: Date().addingTimeInterval(0.2))
        return Set(AccessibilityTree.identifiers(in: window))
    }
}
