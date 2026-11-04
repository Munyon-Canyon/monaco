import XCTest

enum ChatCabalChatJourney {
    static let id = "chat/cabal-chat"
    static let version = 8
    static let quietTitle = "No vote open."

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA chat \(run)" }
    static func message(run: String) -> String { "QA hi \(run)" }

    static func openChat(_ app: XCUIApplication, run: String, step: String) {
        app.openCabal(named: cabal(run: run), tab: "chat", step: step)
        XCTAssertTrue(
            JoinJourney.waitForLabel(app.element("cabal-inline-title"), containing: cabal(run: run), timeout: 15),
            "\(step): the title for \(cabal(run: run)) did not show within 15 s"
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
                JoinJourney.waitForLabel(app.element("chat-quiet-title"), containing: quietTitle, timeout: 10),
                "S1.2: no quiet chat title within 10 s"
            )
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("chat-quiet-line"), containing: "2 members", timeout: 10),
                "S1.2: the quiet chat does not read 2 members"
            )
            XCTAssertTrue(
                app.element("chat-quiet-line").label.contains("$0.00 pot"),
                "S1.2: the quiet chat does not read $0.00 pot: \(app.element("chat-quiet-line").label)")
            for action in ["chat-quiet-propose", "chat-quiet-poll", "chat-quiet-stock"] {
                XCTAssertTrue(app.element(action).waitForExistence(timeout: 5), "S1.2: no \(action) in the quiet chat")
            }
        }

        recorder.step("S1.3", "send a message") {
            let composer = app.element("chat-composer")
            XCTAssertTrue(composer.waitForExistence(timeout: 5), "S1.3: no chat composer")
            XCTAssertTrue(
                composer.placeholderValue?.hasPrefix("Message ") == true,
                "S1.3: the quiet composer placeholder does not name the cabal: \(composer.placeholderValue ?? "")")
            XCTAssertTrue(composer.isEnabled, "S1.3: the composer is disabled")
            composer.tap()
            composer.typeText(message(run: run))
            app.element("chat-send").tap()
            XCTAssertTrue(
                messageText(app, run: run).waitForExistence(timeout: 10),
                "S1.3: no message reading \(message(run: run)) within 10 s"
            )
            XCTAssertEqual(
                composer.placeholderValue, "Message", "S1.3: the composer placeholder is not 'Message' after the send")
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

extension ChatCabalChatJourney {
    static func messageID(_ app: XCUIApplication, run: String) -> String {
        String(messageText(app, run: run).identifier.dropFirst("chat-message-".count))
    }

    static func bReacts(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.5", "B reacts to A's message") {
            let bubble = messageText(app, run: run)
            XCTAssertTrue(bubble.waitForExistence(timeout: 10), "S1.5: no message reading \(message(run: run))")
            let id = messageID(app, run: run)
            bubble.press(forDuration: 1.0)
            XCTAssertTrue(
                app.element("reaction-tray-chat-\(id)").waitForExistence(timeout: 5),
                "S1.5: no reaction-tray-chat-\(id) within 5 s")
            let thumbs = app.element("reaction-tray-1F44D")
            XCTAssertTrue(thumbs.waitForExistence(timeout: 5), "S1.5: no 👍 in the quick tray")
            thumbs.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("chat-tapback-\(id)"), containing: "1 reaction", timeout: 10),
                "S1.5: chat-tapback-\(id) does not read 1 reaction within 10 s")
        }
    }

    static func aSeesTheTapbackAndPins(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.5b", "A sees the reaction on the message") {
            openChat(app, run: run, step: "S1.5b")
            let id = messageID(app, run: run)
            let tapback = app.element("chat-tapback-\(id)")
            XCTAssertTrue(
                JoinJourney.waitForLabel(tapback, containing: "1 reaction", timeout: 10),
                "S1.5b: chat-tapback-\(id) does not read 1 reaction within 10 s")
            XCTAssertTrue(tapback.label.contains("👍"), "S1.5b: the tapback does not show 👍: \(tapback.label)")
        }

        recorder.step("S1.6", "A pins the message") {
            let id = messageID(app, run: run)
            messageText(app, run: run).coordinate(withNormalizedOffset: CGVector(dx: 0.85, dy: 0.9))
                .press(forDuration: 1.0)
            let pin = app.element("chat-action-pin")
            XCTAssertTrue(pin.waitForExistence(timeout: 5), "S1.6: no Pin in the message menu for \(id)")
            pin.tap()
            XCTAssertTrue(
                app.element("chat-pin-capsule").waitForExistence(timeout: 10),
                "S1.6: no chat-pin-capsule within 10 s")
        }
    }

    static func bSeesThePinAndVotes(
        _ app: XCUIApplication, session: JourneySession, run: String, pinner: String, recorder: JourneyRecorder
    ) throws {
        try session.waitForSetup("S1.7")

        recorder.step("S1.6b", "B sees the pin") {
            openChat(app, run: run, step: "S1.6b")
            let next = app.element("chat-pin-next")
            XCTAssertTrue(next.waitForExistence(timeout: 10), "S1.6b: no chat-pin-next within 10 s")
            let first = pinner.split(separator: " ").first.map(String.init) ?? pinner
            XCTAssertTrue(
                JoinJourney.waitForLabel(next, containing: "Pinned by \(first)", timeout: 10),
                "S1.6b: the pin does not read Pinned by \(first): \(next.label)")
            let text = message(run: run)
            XCTAssertTrue(next.label.contains(text), "S1.6b: the pin does not read \(text): \(next.label)")
        }

        recorder.step("S1.7", "B votes yes on the card in the thread") {
            let yesMatch = app.descendants(matching: .any).matching(
                NSPredicate(format: "identifier BEGINSWITH 'chat-proposal-yes-'")
            ).firstMatch
            app.scrollIntoReach(yesMatch)
            XCTAssertTrue(yesMatch.waitForExistence(timeout: 15), "S1.7: no vote card in the thread within 15 s")
            let id = String(yesMatch.identifier.dropFirst("chat-proposal-yes-".count))
            XCTAssertTrue(app.element("chat-proposal-no-\(id)").exists, "S1.7: no chat-proposal-no-\(id)")
            yesMatch.tap()
            app.holdToCommit(yesMatch)
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("chat-proposal-yes-\(id)"), containing: "Voted Yes", timeout: 10),
                "S1.7: chat-proposal-yes-\(id) does not read Voted Yes within 10 s")
            app.waitForToastGone(timeout: 10)
            app.answerVotePermissionSheet(turnOn: false, timeout: 3)
        }
    }
}
