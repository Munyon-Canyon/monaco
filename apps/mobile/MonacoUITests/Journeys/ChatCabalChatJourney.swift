import XCTest

enum ChatCabalChatJourney {
    static let id = "chat/cabal-chat"
    static let version = 6
    static let emptyTitle = "Start the conversation"
    static let emptyHint =
        "Messages here stay with your cabal. Proposal-specific discussion stays attached to the proposal."

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA chat \(run)" }
    static func message(run: String) -> String { "QA hi \(run)" }

    static func openChat(_ app: XCUIApplication, run: String, step: String) {
        app.openCabal(named: cabal(run: run))
        let more = app.element("cabal-more-button")
        XCTAssertTrue(more.waitForExistence(timeout: 10), "\(step): no '…' button on the cabal page")
        more.tap()
        let chat = app.element("cabal-more-chat")
        XCTAssertTrue(chat.waitForExistence(timeout: 5), "\(step): no Chat item in the '…' menu")
        chat.tap()
        XCTAssertTrue(
            JoinJourney.waitForLabel(app.element("chat-title"), containing: cabal(run: run), timeout: 15),
            "\(step): the chat for \(cabal(run: run)) did not show within 15 s"
        )
    }

    static func messageText(_ app: XCUIApplication, run: String) -> XCUIElement {
        text(app, containing: message(run: run))
    }

    static func text(_ app: XCUIApplication, containing text: String) -> XCUIElement {
        app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'chat-message-' AND label CONTAINS %@", text)
        ).firstMatch
    }

    static func aSends(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the cabal's chat") {
            openChat(app, run: run, step: "S1.1")
        }

        recorder.step("S1.2", "the empty chat says hi") {
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("chat-empty"), containing: emptyTitle, timeout: 10)
                    && JoinJourney.waitForLabel(app.element("chat-empty"), containing: emptyHint, timeout: 10),
                "S1.2: no 'Start the conversation' empty state within 10 s"
            )
        }

        recorder.step("S1.3", "send a message") {
            let composer = app.element("chat-composer")
            XCTAssertTrue(composer.waitForExistence(timeout: 5), "S1.3: no chat composer")
            XCTAssertEqual(
                composer.placeholderValue, "Message your cabal", "S1.3: the composer placeholder is not the spec's")
            XCTAssertTrue(composer.isEnabled, "S1.3: the composer is disabled")
            composer.tap()
            composer.typeText(message(run: run))
            app.element("chat-send").tap()
            XCTAssertTrue(
                messageText(app, run: run).waitForExistence(timeout: 10),
                "S1.3: no message reading \(message(run: run)) within 10 s"
            )
        }

        recorder.step("S1.3b", "send twice in a row, then mention a member") {
            let composer = app.element("chat-composer")
            for word in ["two", "three"] {
                composer.tap()
                composer.typeText("QA \(word) \(run)")
                app.element("chat-send").tap()
                XCTAssertTrue(
                    text(app, containing: "QA \(word) \(run)").waitForExistence(timeout: 10),
                    "S1.3b: no message reading QA \(word) \(run) within 10 s"
                )
            }
            composer.tap()
            composer.typeText("QA mention \(run) @")
            let pick = app.descendants(matching: .any).matching(
                NSPredicate(
                    format: "identifier BEGINSWITH 'chat-mention-' AND identifier != 'chat-mention-picker'")
            ).firstMatch
            XCTAssertTrue(pick.waitForExistence(timeout: 5), "S1.3b: no member in the mention picker within 5 s")
            pick.tap()
            app.element("chat-send").tap()
            XCTAssertTrue(
                text(app, containing: "QA mention \(run) @").waitForExistence(timeout: 10),
                "S1.3b: no mention message within 10 s"
            )
            XCTAssertTrue(messageText(app, run: run).exists, "S1.3b: the first message vanished after the sends")
            XCTAssertTrue(app.element("chat-thread").exists, "S1.3b: the chat thread is gone after the sends")
        }
    }

    static func leaveChat(_ app: XCUIApplication) {
        app.element("chat-thread").tap()
        for _ in 0..<4 {
            if app.tab("Home").exists { return }
            let back = app.buttons["BackButton"].firstMatch
            guard back.waitForExistence(timeout: 2) else { return }
            back.tap()
        }
    }

    static func bReceives(_ app: XCUIApplication, run: String, sender: String, recorder: JourneyRecorder) {
        recorder.step("S1.4", "B sees A's message") {
            openChat(app, run: run, step: "S1.4")
            XCTAssertTrue(
                messageText(app, run: run).waitForExistence(timeout: 15),
                "S1.4: no message reading \(message(run: run)) within 15 s"
            )
            let label = messageText(app, run: run).label
            XCTAssertTrue(
                label.hasPrefix(sender), "S1.4: the message does not start with the author \(sender): \(label)")
        }
    }
}
