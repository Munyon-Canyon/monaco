import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@Suite(.serialized)
@MainActor
struct ProposalVoteSectionTests {
    private final class Recorder {
        var landed = 0
        var warnings = 0

        var haptics: VoteHaptics { VoteHaptics(pick: {}, landed: { self.landed += 1 }) }
    }

    private struct Host: View {
        let model: ProposalDetailModel
        let toasts: ToastCenter
        let recorder: Recorder

        var body: some View {
            if let summary = model.summary {
                ProposalVoteSection(
                    model: model, summary: summary, haptics: recorder.haptics,
                    warning: { recorder.warnings += 1 }, viewerID: "viewer-1"
                )
                .environment(toasts)
            }
        }
    }

    private static let retracted =
        #"{"proposal_id":"proposal-1","status":"open","tally":{"yes":4,"no":1,"voters":8,"needed":5}}"#
    private let detailPath = "/v1/proposals/proposal-1"
    private let votesPath = "/v1/proposals/proposal-1/votes"

    private func detail(
        status: Components.Schemas.ProposalStatus = .open, ballot: Components.Schemas.BallotChoice? = nil,
        canVote: Bool = true, yes: Int = 4, no: Int = 1, voters: Int = 8
    ) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var value = Components.Schemas.ProposalDetail.openNVDA()
        value.cabalName = "Pilates"
        value.status = status
        value.canVote = canVote
        value.myBallot = ballot.flatMap { .init(rawValue: $0.rawValue) }
        value.tally = .init(yes: yes, no: no, voters: voters, needed: 5)
        value.replyCount = 6
        return .json(.ok, String(decoding: try encoder.encode(value), as: UTF8.self))
    }

    private func cast(
        status: String = "open", yes: Int, no: Int, undoIn: TimeInterval? = 15, yesOrder: Int = 4, decided: Bool = false
    ) -> StubTransport.Reply {
        let until = undoIn.map { "\"\(ISO8601DateFormatter().string(from: Date().addingTimeInterval($0)))\"" } ?? "null"
        return .json(
            .ok,
            #"{"proposal_id":"proposal-1","status":"\#(status)","tally":{"yes":\#(yes),"no":\#(no),"voters":8,"needed":5},"my_ballot":"yes","undo_until":\#(until),"my_vote":{"order":6,"of":8,"yes_order":\#(yesOrder),"no_order":0,"decided":\#(decided)}}"#
        )
    }

    private func loaded(_ transport: StubTransport) async -> ProposalDetailModel {
        let model = ProposalDetailModel(
            id: "proposal-1", cabalID: "cabal-1",
            repository: ProposalsRepository(
                api: APIClient(
                    serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)),
            hints: FakeHintSource())
        await model.load()
        return model
    }

    private func hold(_ side: String, in window: UIWindow) async throws {
        let button = try #require(VoteTestKit.node("proposal-vote-\(side)", in: window))
        let action = try #require(button.accessibilityCustomActions?.first { $0.name == "Vote \(side.capitalized)" })
        _ = action.actionHandler?(action)
        await VoteTestKit.settle()
    }

    @Test func theCardIsHiddenWhenClosedReadOnlyForANonVoterAndAPairOtherwise() {
        func card(
            open: Bool = true, settled: Bool = false, role: ProposalViewerRole = .voter,
            selection: ProposalVoteSelection = .none, flash: Bool = false
        ) -> ProposalVoteSection.Card {
            ProposalVoteSection.card(
                isOpen: open, isSettled: settled, role: role, selection: selection, showsCommitted: flash)
        }

        #expect(card() == .pair)
        #expect(card(selection: .picked(.yes)) == .pair)
        #expect(card(selection: .committed(.yes)) == .voted(.yes))
        #expect(card(selection: .committed(.yes), flash: true) == .pair)
        #expect(card(open: false) == .hidden)
        #expect(card(open: false, settled: true, selection: .committed(.yes)) == .pair)
        #expect(card(role: .nonMember) == .readOnly)
        #expect(card(role: .memberNotVoter, selection: .committed(.yes)) == .readOnly)
    }

    @Test func theSelectionMapsBothWaysForEverySide() {
        for side in [ProposalBallot.yes, .no] {
            let voteSide: VoteSide = side == .yes ? .yes : .no
            #expect(ProposalVoteSection.selection(ProposalVoteSelection.picked(side)) == .picked(voteSide))
            #expect(ProposalVoteSection.selection(ProposalVoteSelection.pending(side)) == .pending(voteSide))
            #expect(ProposalVoteSection.selection(ProposalVoteSelection.committed(side)) == .committed(voteSide))
            #expect(ProposalVoteSection.selection(VoteSelection.picked(voteSide)) == .picked(side))
            #expect(ProposalVoteSection.selection(VoteSelection.pending(voteSide)) == .pending(side))
            #expect(ProposalVoteSection.selection(VoteSelection.committed(voteSide)) == .committed(side))
        }
        #expect(ProposalVoteSection.selection(ProposalVoteSelection.none) == VoteSelection.none)
        #expect(ProposalVoteSection.selection(VoteSelection.none) == ProposalVoteSelection.none)
    }

    @Test(.timeLimit(.minutes(1)))
    func anOpenVoteDrawsThePairAndAnOrdinaryVoteToastsItsPlaceWithUndoAndOneSuccessHaptic() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(ballot: .yes, yes: 5)],
                votesPath: [cast(yes: 5, no: 1)],
            ])
        let model = await loaded(transport)
        let toasts = ToastCenter()
        let recorder = Recorder()
        let window = try VoteTestKit.window(Host(model: model, toasts: toasts, recorder: recorder))
        defer { TestWindow.close(window) }
        await VoteTestKit.settle()

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.contains("proposal-vote-yes") && identifiers.contains("proposal-vote-no"))
        #expect(!identifiers.contains("proposal-vote-readonly"))
        let yes = try #require(VoteTestKit.node("proposal-vote-yes", in: window))
        #expect(yes.accessibilityLabel == "Yes, 4 votes")

        try await hold("yes", in: window)
        await VoteTestKit.settle { toasts.current != nil }

        #expect(toasts.current?.message == "Voted Yes. 6th of 8.")
        #expect(toasts.current?.action?.title == "Undo")
        #expect(recorder.landed == 1)
        #expect(recorder.warnings == 0)
        #expect(model.voteSelection == .committed(.yes))
        let gets = await transport.sent.filter { $0.path == detailPath }
        #expect(gets.count == 2)
    }

    @Test(.timeLimit(.minutes(1)))
    func aRefusedVoteWarnsOnceReturnsToPickedAndAHoldAgainSendsANewRequest() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(ballot: .yes, yes: 5)],
                votesPath: [.json(.internalServerError, #"{"message":"Try again"}"#), cast(yes: 5, no: 1)],
            ])
        let model = await loaded(transport)
        let toasts = ToastCenter()
        let recorder = Recorder()
        let window = try VoteTestKit.window(Host(model: model, toasts: toasts, recorder: recorder))
        defer { TestWindow.close(window) }
        await VoteTestKit.settle()

        try await hold("yes", in: window)
        await VoteTestKit.settle { recorder.warnings > 0 }

        #expect(recorder.warnings == 1)
        #expect(recorder.landed == 0)
        #expect(model.voteSelection == .picked(.yes))
        #expect(toasts.current?.isSuccess == false)

        try await hold("yes", in: window)
        await VoteTestKit.settle { recorder.landed > 0 }
        #expect(recorder.landed == 1)
        let posts = await transport.sent.filter { $0.path == votesPath }
        #expect(posts.count == 2)
    }

    @Test(.timeLimit(.minutes(1)))
    func theVoteThatReachesQuorumDrawsTheSettledCardAndToastsWithNoUndo() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(status: .passed, ballot: .yes, canVote: false, yes: 5)],
                votesPath: [cast(status: "passed", yes: 5, no: 1, undoIn: nil, yesOrder: 5, decided: true)],
            ])
        let model = await loaded(transport)
        let toasts = ToastCenter()
        let recorder = Recorder()
        let window = try VoteTestKit.window(Host(model: model, toasts: toasts, recorder: recorder))
        defer { TestWindow.close(window) }
        await VoteTestKit.settle()

        try await hold("yes", in: window)
        await VoteTestKit.settle { toasts.current != nil }
        await VoteTestKit.settle()

        #expect(toasts.current?.message == "Voted Yes. Passed 5–1.")
        #expect(toasts.current?.action == nil)
        #expect(recorder.landed == 1)
        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.contains("proposal-vote-settled") && identifiers.contains("proposal-vote-deciding"))
        let yes = try #require(VoteTestKit.node("proposal-vote-yes", in: window))
        #expect(yes.accessibilityLabel == "Voted Yes, 5 votes")
        #expect(!identifiers.contains("proposal-remind") && !identifiers.contains("proposal-change-vote"))
    }

    @Test(.timeLimit(.minutes(1)))
    func tappingUndoSendsOneDeleteAndToastsVoteUndone() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(ballot: .yes, yes: 5), try detail()],
                votesPath: [
                    cast(yes: 5, no: 1),
                    .json(.ok, Self.retracted),
                ],
            ])
        let model = await loaded(transport)
        let toasts = ToastCenter()
        let recorder = Recorder()
        let window = try VoteTestKit.window(Host(model: model, toasts: toasts, recorder: recorder))
        defer { TestWindow.close(window) }
        await VoteTestKit.settle()
        try await hold("yes", in: window)
        await VoteTestKit.settle { toasts.current != nil }

        let undo = try #require(toasts.current?.action)
        undo.perform()
        await VoteTestKit.settle { toasts.current?.message == "Vote undone." }
        await VoteTestKit.settle()

        #expect(toasts.current?.message == "Vote undone.")
        #expect(model.voteSelection == .none)
        let deletes = await transport.sent.filter { $0.path == votesPath && $0.method == .delete }
        #expect(deletes.count == 1)
        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.contains("proposal-vote-yes"))
    }

    @Test(.timeLimit(.minutes(1)))
    func aVotedProposalDrawsTheOrdinaryRowAndChangeReopensThePicker() async throws {
        let transport = StubTransport(routes: [detailPath: [try detail(ballot: .yes, yes: 5)]])
        let model = await loaded(transport)
        let window = try VoteTestKit.window(Host(model: model, toasts: ToastCenter(), recorder: Recorder()))
        defer { TestWindow.close(window) }
        await VoteTestKit.settle()

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.contains("proposal-voted") && identifiers.contains("proposal-change-vote"))
        #expect(!identifiers.contains("proposal-vote-yes"))

        model.beginChange()
        await VoteTestKit.settle()

        #expect(model.voteSelection == .picked(.no))
        let after = AccessibilityTree.identifiers(in: window)
        #expect(after.contains("proposal-vote-yes") && after.contains("proposal-vote-hint"))
    }

    @Test(.timeLimit(.minutes(1)))
    func aNonMemberSeesTheReadOnlyCardWithNoButtonsAndNoRemind() async throws {
        let transport = StubTransport(routes: [detailPath: [try detail(canVote: false, yes: 3, no: 0, voters: 7)]])
        let model = await loaded(transport)
        let window = try VoteTestKit.window(Host(model: model, toasts: ToastCenter(), recorder: Recorder()))
        defer { TestWindow.close(window) }
        await VoteTestKit.settle()

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.contains("proposal-vote-readonly"))
        #expect(!identifiers.contains("proposal-vote-yes") && !identifiers.contains("proposal-vote-no"))
        #expect(!identifiers.contains("proposal-remind") && !identifiers.contains("proposal-change-vote"))
        let card = try #require(VoteTestKit.node("proposal-vote-readonly", in: window))
        #expect(card.accessibilityLabel == "3 yes, 0 no. Pilates members vote. 3 of 7 voted.")
    }

    @Test(.timeLimit(.minutes(1)))
    func aClosedProposalDrawsNothing() async throws {
        let transport = StubTransport(routes: [detailPath: [try detail(status: .failed, canVote: false)]])
        let model = await loaded(transport)
        let window = try VoteTestKit.window(Host(model: model, toasts: ToastCenter(), recorder: Recorder()))
        defer { TestWindow.close(window) }
        await VoteTestKit.settle()

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(!identifiers.contains { $0.hasPrefix("proposal-vote") })
    }
}
