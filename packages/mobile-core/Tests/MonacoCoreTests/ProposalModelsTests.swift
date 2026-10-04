import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class ProposalModelsTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_000_000)
    private let kai = Components.Schemas.ProposalDetail.sampleVoterIDs[0]
    private let jordan = Components.Schemas.ProposalDetail.sampleVoterIDs[1]

    func testDetailNamesEachVoterAndHidesVotingForANonVoter() async throws {
        let proposal = Components.Schemas.ProposalDetail.sample(ballots: [jordan: .yes], canVote: false, now: now)
        let model = detailModel(ProposalsPreviewServer(proposals: [proposal]), id: proposal.id)

        await model.load()

        let screen = try XCTUnwrap(model.screen)
        XCTAssertEqual(screen.voters.map(\.text), ["Kai hasn't voted", "Jordan voted yes", "Priya hasn't voted"])
        XCTAssertEqual(screen.card.ballot, .none)
        XCTAssertEqual(screen.reasonTitle, "Why buy")
        XCTAssertEqual(screen.expected, "about 0.7319 shares at $341.57")
        XCTAssertEqual(screen.steps.reached, 0)
    }

    func testVotingThenChangingSendsTwoBallotsAndToastsEachTime() async throws {
        let proposal = Components.Schemas.ProposalDetail.sample(now: now)
        let server = ProposalsPreviewServer(proposals: [proposal])
        let model = detailModel(server, id: proposal.id)
        await model.load()
        XCTAssertEqual(model.screen?.card.ballot, .ask)

        await model.vote(.yes)
        XCTAssertEqual(model.screen?.card.ballot, .voted(.yes))
        XCTAssertEqual(model.screen?.card.tracker, "1 of 3 voted · 2 yes to pass")
        XCTAssertEqual(model.voting.toast?.message, "Vote in")
        let firstToast = model.voting.toast?.serial

        await model.vote(.no)
        XCTAssertEqual(model.screen?.card.ballot, .voted(.no))
        XCTAssertEqual(model.voting.toast?.message, "Vote in")
        XCTAssertNotEqual(model.voting.toast?.serial, firstToast)
        let posts = await server.count("POST /v1/proposals/\(proposal.id)/votes")
        XCTAssertEqual(posts, 2)
    }

    func testAProposalUpdatedHintRefetchesTheDetailExactlyOnce() async throws {
        let proposal = Components.Schemas.ProposalDetail.sample(now: now)
        let server = ProposalsPreviewServer(proposals: [proposal])
        let hints = FakeHintStream()
        let model = ProposalDetailModel(proposalID: proposal.id, repository: .preview(server), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.cabal("another-cabal"), what: "proposal_updated", id: "1"))
        await hints.send(.changed(.cabal(proposal.cabalId), what: "members", id: "2"))
        await hints.send(.changed(.cabal(proposal.cabalId), what: "proposal_updated", id: "3"))
        let refetched = await waitUntil { await server.count("GET /v1/proposals/") == 2 }
        XCTAssertTrue(refetched)
        for _ in 0..<50 { await Task.yield() }

        let reads = await server.count("GET /v1/proposals/")
        XCTAssertEqual(reads, 2)
    }

    func testABuyReachingDoneCelebratesOnce() async throws {
        let proposal = Components.Schemas.ProposalDetail.sample(status: .passed, now: now)
        let server = ProposalsPreviewServer(proposals: [proposal])
        let model = detailModel(server, id: proposal.id)
        await model.load()
        XCTAssertEqual(model.celebrations, 0)

        await server.replace(.sample(status: .executed, now: now))
        await model.load()
        await model.load()

        XCTAssertEqual(model.celebrations, 1)
        XCTAssertEqual(model.screen?.card.chip, "Bought")
    }

    func testDetailLoadErrorsFailFirstAndToastOnceShown() async throws {
        let proposal = Components.Schemas.ProposalDetail.sample(now: now)
        let server = ProposalsPreviewServer(proposals: [proposal])
        await server.setFailing(true)
        let model = detailModel(server, id: proposal.id)

        await model.load()
        guard case .failed = model.state else { return XCTFail("expected a failed first load, got \(model.state)") }

        await server.setFailing(false)
        await model.load()
        await server.setFailing(true)
        await model.load()
        XCTAssertNotNil(model.screen)
        XCTAssertEqual(model.voting.toast?.isSuccess, false)
    }

    func testAMemberWithNoOpenProposalsSeesTheEmptyState() async throws {
        let model = cabalModel(ProposalsPreviewServer(proposals: []))
        await model.load()
        XCTAssertEqual(model.state, .loaded(.empty))
    }

    func testANonMemberWithNoOpenProposalsSeesNothing() async throws {
        let model = cabalModel(ProposalsPreviewServer(cabal: .sampleWithMembers(role: nil), proposals: []))
        await model.load()
        XCTAssertEqual(model.state, .loaded(.hidden))
    }

    func testCabalCardsAreNewestFirstAndCountTheVotesWaitingOnYou() async throws {
        var older = Components.Schemas.ProposalDetail.sample(id: "p-older", now: now)
        older.createdAt = now.addingTimeInterval(-7200)
        let newer = Components.Schemas.ProposalDetail.sample(id: "p-newer", ballots: [kai: .no], now: now)
        let closed = Components.Schemas.ProposalDetail.sample(id: "p-closed", status: .expired, now: now)
        let model = cabalModel(ProposalsPreviewServer(proposals: [older, newer, closed]))

        await model.load()

        guard case .loaded(.cards(let cards, let awaiting)) = model.state else {
            return XCTFail("expected cards, got \(model.state)")
        }
        XCTAssertEqual(cards.map(\.id), ["p-newer", "p-older"])
        XCTAssertEqual(cards.map(\.ballot), [.voted(.no), .ask])
        XCTAssertEqual(awaiting, 1)
    }

    func testANonMemberSeesReadOnlyCards() async throws {
        let server = ProposalsPreviewServer(cabal: .sampleWithMembers(role: nil), proposals: [.sample(now: now)])
        let model = cabalModel(server)
        await model.load()
        guard case .loaded(.cards(let cards, let awaiting)) = model.state else {
            return XCTFail("expected cards, got \(model.state)")
        }
        XCTAssertEqual(cards.map(\.ballot), [.none])
        XCTAssertEqual(awaiting, 0)
    }

    func testAProposalCreatedHintReloadsTheCabalListOnce() async throws {
        let server = ProposalsPreviewServer(proposals: [])
        let hints = FakeHintStream()
        let cabalID = Components.Schemas.Cabal.sample(role: "member").id
        let model = CabalProposalsModel(cabalID: cabalID, repository: .preview(server), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 2 }

        await server.replace(.sample(now: now))
        await hints.send(.changed(.cabal(cabalID), what: "proposal_created", id: "1"))
        let reloaded = await waitUntil {
            if case .loaded(.cards) = model.state { return true }
            return false
        }
        XCTAssertTrue(reloaded)
        for _ in 0..<50 { await Task.yield() }

        let lists = await server.count("GET /v1/cabals/\(cabalID)/proposals")
        XCTAssertEqual(lists, 2)
    }

    func testHomeShowsThreePendingVotesInServerOrderAndCountsAll() async throws {
        let proposals = (1...4).map { index in
            var proposal = Components.Schemas.ProposalDetail.sample(id: "p-\(index)", now: now)
            proposal.expiresAt = now.addingTimeInterval(TimeInterval(index * 3600))
            return proposal
        }
        let model = PendingVotesModel(
            repository: .preview(ProposalsPreviewServer(proposals: proposals.reversed())),
            limit: PendingVotesModel.homeLimit)

        await model.load()

        guard case .loaded(let section) = model.state else { return XCTFail("expected pending votes") }
        XCTAssertEqual(section.cards.map(\.id), ["p-1", "p-2", "p-3"])
        XCTAssertEqual(section.count, 4)
        XCTAssertTrue(section.cards.allSatisfy { $0.ballot == .ask })
    }

    func testVotingFromHomeDropsTheCard() async throws {
        let proposal = Components.Schemas.ProposalDetail.sample(now: now)
        let model = PendingVotesModel(repository: .preview(ProposalsPreviewServer(proposals: [proposal])), limit: nil)
        await model.load()
        guard case .loaded(let before) = model.state, let card = before.cards.first else {
            return XCTFail("expected a card")
        }

        await model.vote(.yes, on: card)

        guard case .loaded(let after) = model.state else { return XCTFail("expected pending votes") }
        XCTAssertEqual(after.cards, [])
        XCTAssertEqual(after.count, 0)
        XCTAssertEqual(model.voting.toast?.message, "Vote in")
    }

    func testTheListSplitsOpenAndClosed() async throws {
        let open = Components.Schemas.ProposalDetail.sample(id: "p-open", now: now)
        let voided = Components.Schemas.ProposalDetail.sample(id: "p-voided", status: .voided, now: now)
        let model = ProposalListModel(
            cabalID: open.cabalId, repository: .preview(ProposalsPreviewServer(proposals: [open, voided])),
            hints: ProposalsPreviewHints())

        await model.load(.open)
        await model.load(.closed)

        XCTAssertEqual(model.pager(.open).items.map(\.id), ["p-open"])
        XCTAssertEqual(model.pager(.closed).items.map(\.id), ["p-voided"])
        XCTAssertEqual(model.pager(.closed).items.first?.chip, "Voided by Monaco")
    }

    func testVotingFromTheListRefreshesBothTabs() async throws {
        let open = Components.Schemas.ProposalDetail.sample(id: "p-open", now: now)
        let server = ProposalsPreviewServer(proposals: [open])
        let model = ProposalListModel(
            cabalID: open.cabalId, repository: .preview(server), hints: ProposalsPreviewHints())
        await model.load(.open)
        let card = try XCTUnwrap(model.pager(.open).items.first)

        await model.vote(.yes, on: card)

        XCTAssertEqual(model.pager(.open).items.first?.ballot, .voted(.yes))
        XCTAssertEqual(model.voting.toast?.message, "Vote in")
        let lists = await server.count("GET /v1/cabals/\(open.cabalId)/proposals")
        XCTAssertEqual(lists, 3)
    }

    func testAProposalUpdatedHintRefreshesTheListOnceWhileVisible() async throws {
        let open = Components.Schemas.ProposalDetail.sample(id: "p-open", now: now)
        let server = ProposalsPreviewServer(proposals: [open])
        let hints = FakeHintStream()
        let model = ProposalListModel(cabalID: open.cabalId, repository: .preview(server), hints: hints)
        await model.load(.open)
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 2 }
        model.setVisible(true)

        await hints.send(.changed(.cabal(open.cabalId), what: "proposal_updated", id: "1"))
        let refreshed = await waitUntil {
            await server.count("GET /v1/cabals/\(open.cabalId)/proposals") == 3
        }
        XCTAssertTrue(refreshed)
    }

    func testHiddenScreensWaitForVisibilityBeforeRefetching() async throws {
        let proposal = Components.Schemas.ProposalDetail.sample(now: now)
        let server = ProposalsPreviewServer(proposals: [proposal])
        let hints = FakeHintStream()
        let cabal = CabalProposalsModel(cabalID: proposal.cabalId, repository: .preview(server), hints: hints)
        await cabal.load()
        let observer = Task { await cabal.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 2 }

        cabal.setVisible(false)
        await hints.send(.changed(.cabal(proposal.cabalId), what: "proposal_created", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let whileHidden = await server.count("GET /v1/cabals/\(proposal.cabalId)/proposals")
        XCTAssertEqual(whileHidden, 1)

        cabal.setVisible(true)
        let refetched = await waitUntil {
            await server.count("GET /v1/cabals/\(proposal.cabalId)/proposals") == 2
        }
        XCTAssertTrue(refetched)
    }

    private func detailModel(_ server: ProposalsPreviewServer, id: String) -> ProposalDetailModel {
        ProposalDetailModel(proposalID: id, repository: .preview(server), hints: ProposalsPreviewHints())
    }

    private func cabalModel(_ server: ProposalsPreviewServer) -> CabalProposalsModel {
        CabalProposalsModel(
            cabalID: Components.Schemas.Cabal.sample(role: "member").id, repository: .preview(server),
            hints: ProposalsPreviewHints())
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
