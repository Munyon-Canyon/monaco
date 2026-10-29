import XCTest

enum InviteJourney {
    static let id = "cabals/invite"
    static let version = 9

    static let cabalName = "QA pot"

    static func ownCabal(run: String) -> String { "QA mine \(run)" }
    static let inviteeHandle = "qa_b"

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func inviteTiles(_ app: XCUIApplication) -> XCUIElementQuery {
        app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'cabals-invited-'"))
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

    static func openInviteSheet(_ app: XCUIApplication, step: String) {
        app.waitForToastGone()
        app.dismissPushPrePromptIfShown()
        app.openCabalTab("members")
        let invite = app.element("cabal-members-invite")
        if !invite.waitForExistence(timeout: 3) { app.swipeUp() }
        XCTAssertTrue(invite.waitForExistence(timeout: 10), "\(step): no Invite row on the Members tab within 10 s")
        invite.tap()
        XCTAssertTrue(
            app.element("cabal-invite-title").waitForExistence(timeout: 10),
            "\(step): the invite sheet did not show within 10 s"
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
        let gear = app.element("cabal-invite-settings")
        XCTAssertTrue(gear.waitForExistence(timeout: 10), "S2.3: the invite sheet has no gear round")
        gear.tap()
        let byHandle = app.element("cabal-invite-handle-row")
        XCTAssertTrue(byHandle.waitForExistence(timeout: 10), "S2.3: the gear menu has no Invite by handle row")
        byHandle.tap()
    }

    static func acceptAndInvite(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the Cabals tab and find the invited tile") {
            app.tab("Cabals").tap()
            XCTAssertTrue(
                waitForCount(inviteTiles(app), 1, timeout: 10),
                "S1.1: expected one Invited tile on the Cabals tab within 10 s, found \(inviteTiles(app).count)"
            )
            let label = inviteTiles(app).firstMatch.label
            XCTAssertTrue(label.contains(cabalName), "S1.1: the tile does not name \(cabalName): \(label)")
            XCTAssertTrue(label.contains("invited"), "S1.1: the tile does not read Invited: \(label)")
        }

        recorder.step("S1.2", "accept from the tile and land on the cabal") {
            inviteTiles(app).firstMatch.tap()
            let accept = app.buttons["cabal-invite-accept"].firstMatch
            XCTAssertTrue(accept.waitForExistence(timeout: 10), "S1.2: no Accept in the invitation sheet within 10 s")
            accept.tap()
            waitForToast(app, "You're in.", step: "S1.2")
            XCTAssertTrue(
                app.element("cabal-header-name").waitForExistence(timeout: 15),
                "S1.2: the cabal page did not show within 15 s"
            )
        }

        recorder.step("S1.3", "open the invite sheet from the Members tab") {
            openInviteSheet(app, step: "S1.3")
        }

        recorder.step("S1.4", "the sheet offers Share and Copy link, and a member has no gear") {
            XCTAssertTrue(
                app.buttons["share-card-share"].waitForExistence(timeout: 10),
                "S1.4: Share did not show within 10 s"
            )
            XCTAssertTrue(app.buttons["share-card-copy"].exists, "S1.4: no Copy link in the invite sheet")
            XCTAssertFalse(
                app.element("cabal-invite-settings").exists, "S1.4: a member is offered the invite gear")
            XCTAssertFalse(
                app.element("cabal-invite-handle-row").exists, "S1.4: a member is offered Invite by handle")
            let copy = app.buttons["share-card-copy"]
            XCTAssertTrue(copy.isEnabled, "S1.4: Copy link stayed disabled")
            copy.tap()
            waitForToast(app, "Link copied.", step: "S1.4")
            app.buttons["Close"].firstMatch.tap()
        }
    }

    static func creatorInvites(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal the creator made") {
            app.openCabal(named: ownCabal(run: run))
        }

        recorder.step("S2.2", "open the invite sheet from the Members tab") {
            openInviteSheet(app, step: "S2.2")
        }

        recorder.step("S2.3", "open Invite by handle from the gear") {
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
        recorder.step("S2.8", "open the Cabals tab and find exactly one invited tile") {
            app.tab("Cabals").tap()
            XCTAssertTrue(
                waitForCount(inviteTiles(app), 1, timeout: 10),
                "S2.8: expected exactly one Invited tile within 10 s, found \(inviteTiles(app).count)"
            )
            let label = inviteTiles(app).firstMatch.label
            XCTAssertTrue(
                label.contains(ownCabal(run: run)), "S2.8: the tile does not name \(ownCabal(run: run)): \(label)")
        }

        recorder.step("S2.9", "decline it") {
            inviteTiles(app).firstMatch.tap()
            let decline = app.buttons["cabal-invite-decline"].firstMatch
            XCTAssertTrue(decline.waitForExistence(timeout: 10), "S2.9: no Decline in the invitation sheet")
            decline.tap()
            waitForToast(app, "Invite declined.", step: "S2.9")
            XCTAssertTrue(
                waitForCount(inviteTiles(app), 0, timeout: 10),
                "S2.9: the declined invite is still a tile on the Cabals tab"
            )
        }
    }
}
