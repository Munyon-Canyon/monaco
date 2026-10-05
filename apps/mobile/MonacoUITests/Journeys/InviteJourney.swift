import XCTest

enum InviteJourney {
    static let id = "cabals/invite"
    static let version = 3

    static let cabalName = "QA pot"
    static let inviteeHandle = "qa_b"

    static func shareCabal(run: String) -> String { "QA share \(run)" }

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
            app.element("cabal-details-done").waitForExistence(timeout: 10),
            "S1.3: the Cabal details sheet did not show within 10 s"
        )
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
                "S1.2: the cabal screen's Details button did not show within 15 s"
            )
        }

        recorder.step("S1.3", "open the cabal's details") {
            openDetailsAfterToast(app)
            XCTAssertTrue(
                app.element("cabal-invite-member-row").waitForExistence(timeout: 10),
                "S1.3: no Invite someone row in the details within 10 s"
            )
        }

        recorder.step("S1.4", "open Invite someone") {
            app.element("cabal-invite-member-row").tap()
            XCTAssertTrue(
                app.textFields["invite-member-handle-field"].waitForExistence(timeout: 10),
                "S1.4: the handle field did not show within 10 s"
            )
            XCTAssertTrue(
                app.element("invite-member-pending-empty").waitForExistence(timeout: 10),
                "S1.4: the cabal should have no pending invites yet"
            )
        }

        recorder.step("S1.5", "invite a handle no one has") {
            send(app, handle: "@nobody_zz", step: "S1.5")
            waitForToast(app, "No one on Monaco has that handle.", step: "S1.5")
        }

        recorder.step("S1.6", "invite B by handle, with the @ and capitals") {
            send(app, handle: "@QA_B", step: "S1.6")
            waitForToast(app, "Invite sent.", step: "S1.6")
            XCTAssertTrue(
                waitForCount(pendingRows(app), 1, timeout: 10),
                "S1.6: expected one pending invite, found \(pendingRows(app).count)"
            )
            let text = rowText(pendingRows(app).firstMatch)
            XCTAssertTrue(
                text.contains("@\(inviteeHandle)"), "S1.6: the pending row does not read @\(inviteeHandle): \(text)")
            XCTAssertTrue(
                text.contains("Expires in 7 days"), "S1.6: the pending row does not read Expires in 7 days: \(text)")
        }

        recorder.step("S1.7", "revoke the invite") {
            app.buttons["invite-member-revoke-button"].firstMatch.tap()
            waitForToast(app, "Invite revoked.", step: "S1.7")
            XCTAssertTrue(
                app.element("invite-member-pending-empty").waitForExistence(timeout: 10),
                "S1.7: the revoked invite is still listed"
            )
        }

        recorder.step("S1.8", "invite B again") {
            send(app, handle: inviteeHandle, step: "S1.8")
            waitForToast(app, "Invite sent.", step: "S1.8")
            XCTAssertTrue(
                waitForCount(pendingRows(app), 1, timeout: 10),
                "S1.8: expected one pending invite, found \(pendingRows(app).count)"
            )
        }
    }

    static func decline(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.9", "open the Cabals tab and find exactly one invite") {
            app.tab("Cabals").tap()
            XCTAssertTrue(
                waitForCount(inviteRows(app), 1, timeout: 10),
                "S1.9: expected exactly one invite within 10 s, found \(inviteRows(app).count)"
            )
            let text = rowText(inviteRows(app).firstMatch)
            XCTAssertTrue(text.contains(cabalName), "S1.9: the invite does not name \(cabalName): \(text)")
        }

        recorder.step("S1.10", "decline it") {
            app.buttons["cabal-invite-decline"].firstMatch.tap()
            waitForToast(app, "Invite declined.", step: "S1.10")
            XCTAssertTrue(
                waitForCount(inviteRows(app), 0, timeout: 10),
                "S1.10: the declined invite is still on the Cabals tab"
            )
        }
    }

    static func copyAndShareTheCode(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = shareCabal(run: run)
        let copy = app.buttons["cabal-invite-copy"]
        let share = app.buttons["cabal-invite-share"]

        recorder.step("S2.1", "open the cabal by name") {
            JoinJourney.openBySearch(app, name, step: "S2.1")
        }

        recorder.step("S2.2", "open details and read the invite card") {
            app.buttons["cabal-details-button"].tap()
            XCTAssertTrue(
                app.element("cabal-invite-card").waitForExistence(timeout: 10),
                "S2.2: no invite card in the details within 10 s"
            )
            let code = app.staticTexts["cabal-invite-code"].label
            XCTAssertEqual(code.count, 10, "S2.2: the invite code '\(code)' is not 10 characters")
            XCTAssertEqual(copy.label, "Copy code", "S2.2: the copy button's label")
            XCTAssertEqual(share.label, "Share", "S2.2: the share button's label")
        }

        recorder.step("S2.3", "copy the code") {
            copy.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(copy, containing: "Copied", timeout: 2),
                "S2.3: the copy button did not read Copied within 2 s"
            )
        }

        recorder.step("S2.4", "open the share sheet and close it") {
            share.tap()
            XCTAssertTrue(app.dismissShareSheet(), "S2.4: the share sheet did not open within 5 s")
            XCTAssertTrue(
                app.element("cabal-invite-card").waitForExistence(timeout: 5),
                "S2.4: the invite card did not come back after the share sheet closed"
            )
        }
    }
}
