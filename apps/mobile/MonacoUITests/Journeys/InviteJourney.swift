import XCTest

enum InviteJourney {
    static let id = "cabals/invite"
    static let version = 7

    static let cabalName = "QA pot"

    static func ownCabal(run: String) -> String { "QA mine \(run)" }
    static let inviteeHandle = "qa_b"

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func inviteRows(_ app: XCUIApplication) -> XCUIElementQuery {
        app.descendants(matching: .any).matching(identifier: "cabal-invite-row")
    }

    static func pendingRows(_ app: XCUIApplication) -> XCUIElementQuery {
        app.descendants(matching: .any).matching(identifier: "invite-member-pending-row")
    }

    static func waitForToast(_ app: XCUIApplication, _ message: String, step: String) {
        XCTAssertTrue(
            app.staticTexts[message].firstMatch.waitForExistence(timeout: 10),
            "\(step): the toast \"\(message)\" did not show within 10 s"
        )
    }

    static func waitForCount(_ query: XCUIElementQuery, _ count: Int, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if query.count == count { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        } while Date() < deadline
        return query.count == count
    }

    static func rowText(_ row: XCUIElement) -> String {
        row.descendants(matching: .any).allElementsBoundByIndex.map(\.label).filter { !$0.isEmpty }.joined(
            separator: " | ")
    }

    static func send(_ app: XCUIApplication, handle: String, step: String) {
        let field = app.textFields["invite-member-handle-field"]
        XCTAssertTrue(field.waitForExistence(timeout: 10), "\(step): no handle field")
        field.tap()
        let typed = (field.value as? String) ?? ""
        if !typed.isEmpty {
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: typed.count))
        }
        field.typeText(handle)
        let button = app.buttons["invite-member-send-button"]
        XCTAssertTrue(button.isEnabled, "\(step): Send stayed disabled for '\(handle)'")
        button.tap()
    }

    static func openDetailsAfterToast(_ app: XCUIApplication) {
        app.waitForToastGone()
        app.dismissPushPrePromptIfShown()
        app.buttons["cabal-details-button"].tap()
        XCTAssertTrue(
            app.element("cabal-members-row").waitForExistence(timeout: 10),
            "S1.3: the Cabal information screen did not show within 10 s"
        )
    }

    static func closeSheet(_ app: XCUIApplication) {
        app.dismissKeyboard()
        let grabber = app.buttons["Sheet Grabber"].firstMatch
        if grabber.waitForExistence(timeout: 3) {
            grabber.swipeDown()
            _ = grabber.waitForNonExistence(timeout: 5)
        }
    }

    static func openHandleForm(_ app: XCUIApplication) {
        app.element("cabal-invite-member-row").tap()
        let byHandle = app.element("cabal-invite-handle-row")
        if !byHandle.waitForExistence(timeout: 3) { app.element("cabal-invite-cover").swipeUp() }
        XCTAssertTrue(byHandle.waitForExistence(timeout: 10), "S2.3: the invite sheet has no Invite by handle row")
        byHandle.tap()
    }

    static func acceptAndInvite(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the Cabals tab and find the invite") {
            app.tab("Cabals").tap()
            XCTAssertTrue(
                waitForCount(inviteRows(app), 1, timeout: 10),
                "S1.1: expected one invite on the Cabals tab within 10 s, found \(inviteRows(app).count)"
            )
            let text = rowText(inviteRows(app).firstMatch)
            XCTAssertTrue(text.contains(cabalName), "S1.1: the invite does not name \(cabalName): \(text)")
            XCTAssertTrue(text.contains("invited you"), "S1.1: the invite does not say who invited: \(text)")
        }

        recorder.step("S1.2", "accept and land on the cabal") {
            app.buttons["cabal-invite-accept"].firstMatch.tap()
            waitForToast(app, "You're in.", step: "S1.2")
            XCTAssertTrue(
                app.buttons["cabal-details-button"].waitForExistence(timeout: 15),
                "S1.2: the cabal screen's information button did not show within 15 s"
            )
        }

        recorder.step("S1.3", "open the cabal's details") {
            openDetailsAfterToast(app)
            XCTAssertTrue(
                app.element("cabal-invite-member-row").waitForExistence(timeout: 10),
                "S1.3: no Invite to this cabal row in the information screen within 10 s"
            )
        }

        recorder.step("S1.4", "a member is not offered Invite by handle") {
            app.element("cabal-invite-member-row").tap()
            XCTAssertTrue(
                app.buttons["share-card-share"].waitForExistence(timeout: 10),
                "S1.4: the Share cabal sheet did not show within 10 s"
            )
            app.element("cabal-invite-cover").swipeUp()
            XCTAssertTrue(
                app.element("cabal-invite-gain-row").waitForExistence(timeout: 10),
                "S1.4: no Share your gain as an image row in the sheet"
            )
            XCTAssertFalse(
                app.element("cabal-invite-handle-row").exists, "S1.4: a member is offered Invite by handle")
            app.buttons["Close"].firstMatch.tap()
        }
    }

    static func creatorInvites(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal the creator made") {
            JoinJourney.openBySearch(app, ownCabal(run: run), step: "S2.1")
        }

        recorder.step("S2.2", "open the cabal's details") {
            openDetailsAfterToast(app)
            XCTAssertTrue(
                app.element("cabal-invite-member-row").waitForExistence(timeout: 10),
                "S2.2: no Invite to this cabal row in the information screen within 10 s"
            )
        }

        recorder.step("S2.3", "open Invite someone") {
            openHandleForm(app)
            XCTAssertTrue(
                app.textFields["invite-member-handle-field"].waitForExistence(timeout: 10),
                "S2.3: the handle field did not show within 10 s"
            )
            XCTAssertTrue(
                app.element("invite-member-pending-empty").waitForExistence(timeout: 10),
                "S2.3: the cabal should have no pending invites yet"
            )
        }

        recorder.step("S2.4", "invite a handle no one has") {
            send(app, handle: "@nobody_zz", step: "S2.4")
            waitForToast(app, "No one on Monaco has that handle.", step: "S2.4")
        }

        recorder.step("S2.5", "invite B by handle, with the @ and capitals") {
            send(app, handle: "@QA_B", step: "S2.5")
            waitForToast(app, "Invite sent.", step: "S2.5")
            XCTAssertTrue(
                waitForCount(pendingRows(app), 1, timeout: 10),
                "S2.5: expected one pending invite, found \(pendingRows(app).count)"
            )
            let text = rowText(pendingRows(app).firstMatch)
            XCTAssertTrue(
                text.contains("@\(inviteeHandle)"), "S2.5: the pending row does not read @\(inviteeHandle): \(text)")
            XCTAssertTrue(
                text.contains("Expires in 7 days"), "S2.5: the pending row does not read Expires in 7 days: \(text)")
        }

        recorder.step("S2.6", "revoke the invite") {
            app.buttons["invite-member-revoke-button"].firstMatch.tap()
            waitForToast(app, "Invite revoked.", step: "S2.6")
            XCTAssertTrue(
                app.element("invite-member-pending-empty").waitForExistence(timeout: 10),
                "S2.6: the revoked invite is still listed"
            )
        }

        recorder.step("S2.7", "invite B again") {
            send(app, handle: inviteeHandle, step: "S2.7")
            waitForToast(app, "Invite sent.", step: "S2.7")
            XCTAssertTrue(
                waitForCount(pendingRows(app), 1, timeout: 10),
                "S2.7: expected one pending invite, found \(pendingRows(app).count)"
            )
        }
        closeSheet(app)
    }

    static func decline(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.8", "open the Cabals tab and find exactly one invite") {
            app.tab("Cabals").tap()
            XCTAssertTrue(
                waitForCount(inviteRows(app), 1, timeout: 10),
                "S2.8: expected exactly one invite within 10 s, found \(inviteRows(app).count)"
            )
            let text = rowText(inviteRows(app).firstMatch)
            XCTAssertTrue(
                text.contains(ownCabal(run: run)), "S2.8: the invite does not name \(ownCabal(run: run)): \(text)")
        }

        recorder.step("S2.9", "decline it") {
            app.buttons["cabal-invite-decline"].firstMatch.tap()
            waitForToast(app, "Invite declined.", step: "S2.9")
            XCTAssertTrue(
                waitForCount(inviteRows(app), 0, timeout: 10),
                "S2.9: the declined invite is still on the Cabals tab"
            )
        }
    }
}
