import XCTest

enum CabalsDiscoverJourney {
    static let id = "cabals/discover"
    static let version = 1

    static let screenTimeout: TimeInterval = 15
    static let browseCaption = "Browsing every public cabal opens soon. Search by name to find more."

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabalName(run: String) -> String { "QA discover \(run)" }

    static func emptyStates(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open Discover cabals with a name nobody has") {
            app.tab("Cabals").tap()
            JoinJourney.openDiscover(app, step: "S1.1")
            XCTAssertTrue(
                JoinJourney.searchField(app).waitForExistence(timeout: 10), "S1.1: no search field on Discover cabals")
            XCTAssertTrue(
                app.element("cabals-search-empty").waitForExistence(timeout: 10),
                "S1.1: no cabals-search-empty for a name nobody has")
            XCTAssertEqual(
                app.element("cabals-search-empty").label, "No matching cabals",
                "S1.1: the empty result is not the spec's copy")
        }

        recorder.step("S1.2", "clear the search") {
            let field = JoinJourney.searchField(app)
            let text = field.value as? String ?? ""
            if !text.isEmpty, text != field.placeholderValue {
                field.coordinate(withNormalizedOffset: CGVector(dx: 0.97, dy: 0.5)).tap()
                field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: text.count))
            }
            let caption = app.element("cabals-browse-coming")
            XCTAssertTrue(caption.waitForExistence(timeout: 10), "S1.2: no cabals-browse-coming for an empty query")
            XCTAssertEqual(caption.label, browseCaption, "S1.2: the caption reads '\(caption.label)'")
            XCTAssertFalse(app.element("cabals-search-empty").exists, "S1.2: the empty result is still there")
        }
    }

    static func opensAndAsks(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = cabalName(run: run)

        recorder.step("S2.1", "search for the public cabal") {
            JoinJourney.search(app, for: name, step: "S2.1")
            CabalsBrowseJourney.assertResult(app, named: name, reads: "1 member", step: "S2.1")
            XCTAssertTrue(
                CabalsBrowseJourney.enterButton(app, label: "Request to join \(name)").waitForExistence(timeout: 10),
                "S2.1: no Request on the row")
        }

        recorder.step("S2.2", "open the public cabal") {
            JoinJourney.result(app, named: name).firstMatch.tap()
            XCTAssertTrue(app.element("public-cabal").waitForExistence(timeout: screenTimeout), "S2.2: no public cabal")
            XCTAssertTrue(app.navigationBars[name].exists, "S2.2: the screen is not titled \(name)")
            let joining = app.element("public-cabal-joining")
            XCTAssertTrue(joining.waitForExistence(timeout: 10), "S2.2: no Membership card")
            XCTAssertTrue(joining.label.contains("Approval required"), "S2.2: Joining reads '\(joining.label)'")
            XCTAssertTrue(
                app.element("public-cabal-return").label.contains("Return unavailable"),
                "S2.2: the return block reads '\(app.element("public-cabal-return").label)'")
            let join = app.element("cabal-join-button")
            XCTAssertTrue(join.waitForExistence(timeout: 10), "S2.2: no call to action")
            XCTAssertEqual(join.label, "Request to join", "S2.2: the call to action reads '\(join.label)'")
            XCTAssertFalse(app.element("cabal-action-fund").exists, "S2.2: a non-member sees the member actions")
        }

        recorder.step("S2.3", "ask to join") {
            app.element("cabal-join-button").tap()
            JoinJourney.waitForToast(app, "Request sent. You'll be in once the creator says yes.", step: "S2.3")
            let requested = app.element("cabal-join-requested")
            XCTAssertTrue(requested.waitForExistence(timeout: screenTimeout), "S2.3: no cabal-join-requested")
            XCTAssertTrue(requested.label.contains("Request pending"), "S2.3: the state reads '\(requested.label)'")
            XCTAssertTrue(app.element("cabal-join-cancel").exists, "S2.3: no Cancel request")
        }

        recorder.step("S2.4", "back on Discover the row is pending") {
            app.waitForToastGone()
            app.dismissPushPrePromptIfShown(timeout: 5)
            app.tapBack()
            let pending = app.element("cabals-search-requested")
            if !pending.waitForExistence(timeout: 3) {
                let list = app.element("cabals-search")
                list.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.3))
                    .press(
                        forDuration: 0.1,
                        thenDragTo: list.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.8)))
            }
            XCTAssertTrue(
                pending.waitForExistence(timeout: 10),
                "S2.4: the row does not read Request pending")
            let notice = app.element("cabals-search-pending-notice")
            XCTAssertTrue(notice.waitForExistence(timeout: 10), "S2.4: no pending notice above the list")
            let text = notice.staticTexts.firstMatch
            XCTAssertTrue(
                JoinJourney.waitForLabel(text, containing: "Your request to join \(name) is pending.", timeout: 5),
                "S2.4: the notice reads '\(text.label)'")
        }
    }
}
