import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

extension CabalEditModelTests {
    func testRuleEditsNeverReachTheRequest() async throws {
        let (model, transport, _) = make([.json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: nil))])
        await model.load()
        var edited = try XCTUnwrap(model.settings)
        edited.voters = .list([Self.jordanID])
        edited.threshold = "majority"
        edited.proposalExpirySeconds = 3600
        edited.joinPolicy = .open

        let outcome = await model.save(edited)

        XCTAssertEqual(outcome, .unchanged)
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testANameSaveRetriedAfterADroppedConnectionReusesTheIdempotencyKey() async throws {
        let (model, transport, _) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator, voters: nil)),
            .failure(URLError(.networkConnectionLost)),
            .json(.ok, Self.cabal(name: "QA pot 2", me: Self.creator, voters: nil)),
        ])
        await model.load()

        let first = await saveName(model, "QA pot 2")
        let second = await saveName(model, "QA pot 2")

        XCTAssertEqual(first, .failed("You're offline. Try again."))
        XCTAssertEqual(second, .saved)
        let keyHeader = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.dropFirst().map { $0.headerFields[keyHeader] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertNotNil(keys[0])
        XCTAssertEqual(keys[0], keys[1])
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

    func testThePreviewSavesAnEditedName() async throws {
        let model = CabalEditModel.preview(.sample(role: "creator"))
        await model.load()
        XCTAssertTrue(model.isCreator)
        var edited = try XCTUnwrap(model.settings)
        edited.name = "QA pot 2"

        let outcome = await model.save(edited)

        XCTAssertEqual(outcome, .saved)
        XCTAssertEqual(model.settings, edited)
    }

    func testTheSampleForANonMemberHasNoMembership() async {
        let model = CabalEditModel.preview(.sample(role: nil))
        await model.load()

        XCTAssertFalse(model.isCreator)
    }

    private func saveName(_ model: CabalEditModel, _ name: String) async -> CabalEditModel.SaveOutcome {
        guard var edited = model.settings else { return .unchanged }
        edited.name = name
        return await model.save(edited)
    }
}
