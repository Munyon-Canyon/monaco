import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
enum VoteTestKit {
    final class Recorder {
        var picks = 0
        var reached = 0
        var landed = 0

        var haptics: VoteHaptics {
            VoteHaptics(pick: { self.picks += 1 }, reached: { self.reached += 1 }, landed: { self.landed += 1 })
        }
    }

    @Observable
    final class Box {
        var selection: VoteSelection
        var commits: [VoteSide] = []

        init(_ selection: VoteSelection) { self.selection = selection }
    }

    struct Host: View {
        @Bindable var box: Box
        let recorder: Recorder

        var body: some View {
            VotePair(
                yes: 4, no: 1, selection: $box.selection, isEnabled: true, identifierPrefix: "proposal",
                onCommit: { box.commits.append($0) }, haptics: recorder.haptics)
        }
    }

    static func size(of view: some View, size: DynamicTypeSize) -> CGSize {
        UIHostingController(rootView: view.environment(\.dynamicTypeSize, size))
            .sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude))
    }

    static func window(_ view: some View) throws -> UIWindow {
        try AccessibilityTree.setAutomation(enabled: true)
        let scene = try #require(UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first)
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(rootView: view)
        window.makeKeyAndVisible()
        return window
    }

    static func identifiers(_ view: some View) async throws -> [String] {
        let window = try window(view)
        defer { window.isHidden = true }
        await settle()
        return AccessibilityTree.identifiers(in: window)
    }

    static func spin() {
        RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.01))
    }

    static func settle(until condition: (() -> Bool)? = nil) async {
        for _ in 0..<(condition == nil ? 20 : 200) {
            if condition?() == true { return }
            spin()
            await Task.yield()
        }
    }

    static func nodes(_ node: NSObject, depth: Int = 0) -> [NSObject] {
        guard depth < 80 else { return [] }
        var children: [NSObject] = (node as? UIView)?.subviews ?? []
        if let elements = node.accessibilityElements as? [NSObject] {
            children += elements
        } else {
            let count = node.accessibilityElementCount()
            if count != NSNotFound, count > 0 {
                children += (0..<count).compactMap { node.accessibilityElement(at: $0) as? NSObject }
            }
        }
        return [node] + children.flatMap { nodes($0, depth: depth + 1) }
    }

    static func node(_ identifier: String, in window: UIWindow) -> NSObject? {
        nodes(window).first { node in
            node.responds(to: #selector(getter: UIAccessibilityIdentification.accessibilityIdentifier))
                && (node.value(forKey: "accessibilityIdentifier") as? String) == identifier
        }
    }
}

@MainActor
struct VoteTallyTests {
    @Test func theBarFillsYesFromTheLeftAndNoFromTheRight() {
        let tally = VoteTallyBar.fractions(yes: 4, no: 1, eligible: 8)
        let empty = VoteTallyBar.fractions(yes: 0, no: 0, eligible: 8)
        let full = VoteTallyBar.fractions(yes: 8, no: 0, eligible: 8)

        #expect(tally.yes == 0.5 && tally.no == 0.125)
        #expect(empty.yes == 0 && empty.no == 0)
        #expect(full.yes == 1 && full.no == 0)
        #expect(VoteTallyBar.fractions(yes: 3, no: 2, eligible: 0) == (0, 0))
    }

    @Test func bothEndsAreNeutralGreyAndTheWinnerTakesThePrimaryText() {
        #expect(VoteTallyBar.fill(.yes, winner: nil) == MonacoTheme.secondaryText)
        #expect(VoteTallyBar.fill(.no, winner: nil) == MonacoTheme.secondaryText)
        #expect(VoteTallyBar.fill(.yes, winner: .yes) == MonacoTheme.primaryText)
        #expect(VoteTallyBar.fill(.no, winner: .yes) == MonacoTheme.secondaryText)
    }

    @Test func aHoldShortOfTheEndDoesNotCommitAndTheEndCommitsOnce() {
        var machine = VoteHoldMachine()
        var selection = VoteSelection.none

        #expect(machine.press(.yes, selection: &selection) == [.picked(.yes)])
        #expect(machine.advance(to: 0.59, selection: &selection).isEmpty)
        #expect(selection == .picked(.yes))
        #expect(machine.advance(to: 1.0, selection: &selection) == [.reached(.yes)])
        #expect(selection == .pending(.yes))
        #expect(machine.advance(to: 1.0, selection: &selection).isEmpty)
        #expect(machine.holding == nil)
    }

    @Test func liftingBeforeTheEndKeepsThePickAndResetsTheProgress() {
        var machine = VoteHoldMachine()
        var selection = VoteSelection.picked(.no)

        _ = machine.press(.no, selection: &selection)
        _ = machine.advance(to: 0.4, selection: &selection)
        machine.lift()

        #expect(selection == .picked(.no))
        #expect(machine.progress == 0 && machine.holding == nil)
        #expect(
            VotePair.buttonPhase(.no, selection: selection, holding: nil, progress: 0, isEnabled: true) == .picked)
    }

    @Test func switchingThePickMovesTheStrokeAndDimsTheOther() {
        var machine = VoteHoldMachine()
        var selection = VoteSelection.picked(.yes)

        #expect(machine.pick(.no, selection: &selection) == [.picked(.no)])

        func phase(_ side: VoteSide) -> VoteHoldPhase {
            VotePair.buttonPhase(side, selection: selection, holding: nil, progress: 0, isEnabled: true)
        }
        #expect(phase(.no) == .picked && phase(.yes) == .dimmed)
    }

    @Test func nothingIsHighlightedInIdle() {
        for side in [VoteSide.yes, .no] {
            let phase = VotePair.buttonPhase(side, selection: .none, holding: nil, progress: 0, isEnabled: true)
            #expect(phase == .idle)
        }
    }

    @Test func aPendingVoteIgnoresEveryFurtherTouchAndNamedAction() {
        var machine = VoteHoldMachine()
        var selection = VoteSelection.picked(.yes)
        _ = machine.press(.yes, selection: &selection)
        #expect(machine.advance(to: 1, selection: &selection) == [.reached(.yes)])

        #expect(machine.press(.no, selection: &selection).isEmpty)
        #expect(machine.perform(.no, selection: &selection).isEmpty)
        #expect(machine.pick(.no, selection: &selection).isEmpty)
        #expect(selection == .pending(.yes))
        let phase = VotePair.buttonPhase(
            .yes, selection: selection, holding: nil, progress: 0, isEnabled: true)
        #expect(phase == .holding(progress: 1))
    }

    @Test func aRefusedVoteReturnsToPickedAndTheNextHoldCommitsAgain() {
        var machine = VoteHoldMachine()
        var selection = VoteSelection.picked(.yes)
        var commits = 0
        for effect in machine.press(.yes, selection: &selection) + machine.advance(to: 1, selection: &selection) {
            if case .reached = effect { commits += 1 }
        }
        #expect(commits == 1 && selection == .pending(.yes))

        selection = .picked(.yes)
        let retry = machine.press(.yes, selection: &selection) + machine.advance(to: 1, selection: &selection)

        #expect(retry == [.reached(.yes)])
        #expect(selection == .pending(.yes))
    }

    @Test func aPreselectedSideShowsPickedAndAHoldCommitsIt() {
        var machine = VoteHoldMachine()
        var selection = VoteSelection.picked(.yes)

        let yes = VotePair.buttonPhase(.yes, selection: selection, holding: nil, progress: 0, isEnabled: true)
        let no = VotePair.buttonPhase(.no, selection: selection, holding: nil, progress: 0, isEnabled: true)
        let pressed = machine.press(.yes, selection: &selection)

        #expect(yes == .picked && no == .dimmed)
        #expect(pressed.isEmpty)
        #expect(machine.advance(to: 1, selection: &selection) == [.reached(.yes)])
    }

    @Test func changingAVoteRepicksTheOtherSideAndCommitsThroughPending() {
        var machine = VoteHoldMachine()
        var selection = VoteSelection.committed(.yes)
        #expect(machine.press(.no, selection: &selection).isEmpty)

        selection = .picked(.no)
        _ = machine.press(.no, selection: &selection)
        #expect(machine.advance(to: 1, selection: &selection) == [.reached(.no)])
        #expect(selection == .pending(.no))
        #expect(VoteHaptics.playsLanded(from: .pending(.no), to: .committed(.no)))
    }

    @Test func theNamedActionCommitsWithoutAHoldAndNeverTwice() {
        var machine = VoteHoldMachine()
        var selection = VoteSelection.none

        #expect(machine.perform(.no, selection: &selection) == [.performed(.no)])
        #expect(selection == .pending(.no))
        #expect(machine.perform(.no, selection: &selection).isEmpty)
    }

    @Test func theSuccessHapticPlaysOnlyWhenTheSelectionLandsAsCommitted() {
        #expect(VoteHaptics.playsLanded(from: .pending(.yes), to: .committed(.yes)))
        #expect(VoteHaptics.playsLanded(from: .committed(.yes), to: .committed(.no)))
        #expect(!VoteHaptics.playsLanded(from: .pending(.yes), to: .picked(.yes)))
        #expect(!VoteHaptics.playsLanded(from: .none, to: .picked(.yes)))
        #expect(!VoteHaptics.playsLanded(from: .committed(.yes), to: .committed(.yes)))
    }
}

@MainActor
struct VoteTallyViewTests {
    @Test func aPairThatFirstRendersCommittedPlaysNothingAndALandingPlaysOnce() async throws {
        let recorder = VoteTestKit.Recorder()
        let first = try VoteTestKit.window(VoteTestKit.Host(box: VoteTestKit.Box(.committed(.yes)), recorder: recorder))
        defer { first.isHidden = true }
        await VoteTestKit.settle()
        #expect(recorder.landed == 0 && recorder.picks == 0)

        let box = VoteTestKit.Box(.pending(.yes))
        let second = try VoteTestKit.window(VoteTestKit.Host(box: box, recorder: recorder))
        defer { second.isHidden = true }
        await VoteTestKit.settle()
        box.selection = .committed(.yes)
        await VoteTestKit.settle { recorder.landed > 0 }
        await VoteTestKit.settle()

        #expect(recorder.landed == 1)
    }

    @Test func identifiersFollowThePrefixAndSuffix() async throws {
        let proposal = try await VoteTestKit.identifiers(
            VotePair(
                yes: 4, no: 1, selection: .constant(.none), isEnabled: true, identifierPrefix: "proposal",
                onCommit: { _ in }))
        let home = try await VoteTestKit.identifiers(
            VotePair(
                yes: 4, no: 1, selection: .constant(.none), isEnabled: true, identifierPrefix: "home",
                identifierSuffix: "p1", onCommit: { _ in }))

        #expect(proposal.contains("proposal-vote-yes") && proposal.contains("proposal-vote-no"))
        #expect(proposal.contains("proposal-vote-pair"))
        #expect(home.contains("home-vote-yes-p1") && home.contains("home-vote-no-p1"))
        #expect(!home.contains("vote-yes-button"))
        #expect(VoteIdentifier.make("proposal", "vote-status", nil) == "proposal-vote-status")
        #expect(VoteIdentifier.make("home", "vote-ends", "p1") == "home-vote-ends-p1")
        #expect(VoteIdentifier.make("home", "vote-sides", "p1") == "home-vote-sides-p1")
    }

    @Test func theNonMemberCardDrawsItsPartsAndNoVoteButtons() async throws {
        let card = VStack {
            VoteTallyBar(yes: 3, no: 0, eligible: 7)
            VoteTallyEnds(yes: 3, no: 0, middle: "Pilates members vote", identifierPrefix: "post")
            VoteStatusLine(
                result: nil, facts: ["3 of 7 voted", "2 replies", "ends in 23h"], identifierPrefix: "post")
        }
        let identifiers = try await VoteTestKit.identifiers(card)
        let labels = try await RenderedView.labels(card)

        #expect(identifiers.contains("post-vote-ends") && identifiers.contains("post-vote-status"))
        #expect(!identifiers.contains { $0.contains("vote-yes") })
        #expect(labels.contains("3 of 7 voted, 2 replies, ends in 23h"))
        #expect(labels.contains("3 yes, 0 no, Pilates members vote"))
    }

    @Test func eachButtonIsSpokenWithItsCountAndTheNamedActionsCommit() async throws {
        let box = VoteTestKit.Box(.none)
        let recorder = VoteTestKit.Recorder()
        let window = try VoteTestKit.window(VoteTestKit.Host(box: box, recorder: recorder))
        defer { window.isHidden = true }
        await VoteTestKit.settle()

        let yes = try #require(VoteTestKit.node("proposal-vote-yes", in: window))
        let no = try #require(VoteTestKit.node("proposal-vote-no", in: window))
        #expect(yes.accessibilityLabel == "Yes, 4 votes")
        #expect(no.accessibilityLabel == "No, 1 vote")
        #expect(yes.accessibilityHint == "Pick, then hold to vote")
        let actions = (yes.accessibilityCustomActions ?? []).map(\.name)
        #expect(actions.contains("Vote Yes"))

        let action = try #require(yes.accessibilityCustomActions?.first { $0.name == "Vote Yes" })
        _ = action.actionHandler?(action)
        await VoteTestKit.settle()

        #expect(box.commits == [.yes])
        #expect(box.selection == .pending(.yes))
    }

    @Test func aCommittedButtonReadsVotedAndOffersNoActions() async throws {
        let window = try VoteTestKit.window(
            VoteTestKit.Host(box: VoteTestKit.Box(.committed(.yes)), recorder: VoteTestKit.Recorder()))
        defer { window.isHidden = true }
        await VoteTestKit.settle()

        let yes = try #require(VoteTestKit.node("proposal-vote-yes", in: window))

        #expect(yes.accessibilityLabel == "Voted Yes, 4 votes")
        #expect((yes.accessibilityCustomActions ?? []).isEmpty)
    }

    @Test func thePairAndTheSidesRowFitAtTheLargestStandardAccessibilitySize() {
        let pair = VotePair(
            yes: 4, no: 1, selection: .constant(.none), isEnabled: true, identifierPrefix: "proposal",
            onCommit: { _ in })
        let people = ["Mia", "Christelle", "Logan", "Sam", "You"].map {
            MonacoAvatar.Source(id: $0, displayName: $0, photoURL: nil)
        }
        let sides = VoteSidesRow(
            lines: [.init(side: .yes, avatars: people, text: "Mia, Christelle, Logan, Sam, you")],
            identifierPrefix: "proposal")

        let normal = VoteTestKit.size(of: pair, size: .large)
        let large = VoteTestKit.size(of: pair, size: .accessibility3)
        let row = VoteTestKit.size(of: sides, size: .accessibility3)

        #expect(normal.height >= VoteHoldButton.height && normal.height < 2 * VoteHoldButton.height)
        #expect(large.height >= 2 * VoteHoldButton.height)
        #expect(large.width <= 390 && row.width <= 390)
    }

    @Test func theSidesRowSpeaksEachLineAsOneElement() {
        let yes = VoteSidesRow.Line(side: .yes, avatars: [], text: "Mia, Christelle, Logan, Sam, you")
        let none = VoteSidesRow.Line(side: nil, avatars: [], text: "Jordan and Priya did not vote")

        #expect(VoteSidesRow.spokenLabel(yes) == "Voted yes: Mia, Christelle, Logan, Sam, you.")
        #expect(VoteSidesRow.spokenLabel(none) == "Jordan and Priya did not vote.")
    }
}
