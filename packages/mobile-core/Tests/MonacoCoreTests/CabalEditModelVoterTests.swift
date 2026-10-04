import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

extension CabalEditModelTests {
    func testSaveVotersSendsOnePatchNamingTheCreatorFirst() async throws {
        let (model, transport, _) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: nil)),
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: [Self.creatorID, Self.jordanID])),
        ])
        await model.load()

        let outcome = await model.saveVoters(.list([Self.jordanID]))

        XCTAssertEqual(outcome, .saved)
        XCTAssertEqual(model.voterChoice, .list([Self.creatorID, Self.jordanID]))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .patch])
        let keyHeader = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertNotNil(sent.last?.headerFields[keyHeader])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.last ?? nil)
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? NSDictionary)
        XCTAssertEqual(json, ["voter_mode": "list", "voter_ids": [Self.creatorID, Self.jordanID]])
    }

    func testAVoterSaveRetriedAfterADroppedConnectionReusesTheIdempotencyKey() async throws {
        let (model, transport, _) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: nil)),
            .failure(URLError(.networkConnectionLost)),
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: [Self.creatorID, Self.jordanID])),
        ])
        await model.load()

        let first = await model.saveVoters(.list([Self.jordanID]))
        let second = await model.saveVoters(.list([Self.jordanID]))

        XCTAssertEqual(first, .failed("You're offline. Try again."))
        XCTAssertEqual(second, .saved)
        let keyHeader = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.dropFirst().map { $0.headerFields[keyHeader] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertNotNil(keys[0])
        XCTAssertEqual(keys[0], keys[1])
    }

    func testSavingTheVotersTheCabalAlreadyHasSendsNoRequest() async {
        let (model, transport, _) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: [Self.creatorID, Self.jordanID]))
        ])
        await model.load()

        let sameList = await model.saveVoters(.list([Self.jordanID]))

        XCTAssertEqual(sameList, .unchanged)
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testSavingEveryoneOnACabalWhereEveryoneVotesSendsNoRequest() async {
        let (model, transport, _) = make([.json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: nil))])
        await model.load()

        let outcome = await model.saveVoters(.everyone)

        XCTAssertEqual(outcome, .unchanged)
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testAVoterWhoLeftBeforeTheSaveReloadsTheCabal() async throws {
        let problem =
            #"{"type":"about:blank","title":"Invalid input","status":422,"code":"invalid_input","#
            + #""message":"Every voter must be a member.","trace_id":"00000000000000000000000000000000","#
            + #""retryable":false}"#
        let (model, transport, _) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: nil)),
            .response(status: .unprocessableContent, contentType: "application/problem+json", body: Data(problem.utf8)),
            .json(
                .ok,
                Self.cabal(name: "QA pot", me: Self.creator, voters: nil, members: [Self.creatorID, Self.priyaID])
            ),
        ])
        await model.load()

        let outcome = await model.saveVoters(.list([Self.jordanID]))

        XCTAssertEqual(outcome, .failed("Someone you picked isn't in the cabal anymore."))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .patch, .get])
        XCTAssertEqual(model.cabal?.members.map(\.userId), [Self.creatorID, Self.priyaID])
    }

    func testAMembersHintForThisCabalReadsItAgain() async {
        let (model, transport, hints) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: nil)),
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: nil, members: [Self.creatorID])),
        ])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 2 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.cabal(Self.cabalID), what: "members", id: "1"))

        let refreshed = await waitUntil { model.cabal?.members.count == 1 }
        XCTAssertTrue(refreshed)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testThePreviewAppliesAVoterListToTheMembers() async throws {
        let model = CabalEditModel.preview(.sampleWithMembers(role: "creator"))
        await model.load()
        let members = try XCTUnwrap(model.cabal?.members)

        let outcome = await model.saveVoters(.list([members[1].userId]))

        XCTAssertEqual(outcome, .saved)
        XCTAssertEqual(model.cabal?.rules.voterMode, "list")
        XCTAssertEqual(model.cabal?.members.map(\.canVote), [true, true, false])
        XCTAssertEqual(model.voterChoice, .list([members[0].userId, members[1].userId]))
    }

    func testAHiddenSheetWaitsUntilItIsVisibleToReadAgain() async {
        let (model, transport, hints) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator)),
            .json(.ok, Self.cabal(name: "QA pot 2", me: Self.creator)),
        ])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 2 }
        model.setVisible(false)

        await hints.send(.changed(.cabal(Self.cabalID), what: "updated", id: "1"))
        _ = await waitUntil { false }
        let whileHidden = await transport.sent.count
        model.setVisible(true)

        XCTAssertEqual(whileHidden, 1)
        let refreshed = await waitUntil { model.cabal?.name == "QA pot 2" }
        XCTAssertTrue(refreshed)
    }

    func testThePreviewSavesEveryChangedRule() async throws {
        let model = CabalEditModel.preview(.sample(role: "creator"))
        await model.load()
        XCTAssertTrue(model.isCreator)
        let edited = CabalSettings(
            name: "QA pot 2", joinMode: "request", threshold: "majority", proposalExpirySeconds: 3600)

        let outcome = await model.save(edited)

        XCTAssertEqual(outcome, .saved)
        XCTAssertEqual(model.settings, edited)
    }

    func testTheSampleForANonMemberHasNoMembershipOrInviteCode() async {
        let model = CabalEditModel.preview(.sample(role: nil))
        await model.load()

        XCTAssertFalse(model.isCreator)
        XCTAssertNil(model.cabal?.inviteCode)
    }

    func testAFormSaveAfterAVoterSaveLeavesTheVotersAlone() async throws {
        let (model, transport, _) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: nil)),
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: [Self.creatorID, Self.jordanID])),
            .json(.ok, Self.cabal(name: "QA pot 2", me: Self.creator, voters: [Self.creatorID, Self.jordanID])),
        ])
        await model.load()
        var openedBeforeTheVoterSave = try XCTUnwrap(model.settings)
        _ = await model.saveVoters(.list([Self.jordanID]))
        openedBeforeTheVoterSave.name = "QA pot 2"

        let outcome = await model.save(openedBeforeTheVoterSave)

        XCTAssertEqual(outcome, .saved)
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.last ?? nil)
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
        XCTAssertEqual(json, ["name": "QA pot 2"])
        XCTAssertEqual(model.voterChoice, .list([Self.creatorID, Self.jordanID]))
    }
}
