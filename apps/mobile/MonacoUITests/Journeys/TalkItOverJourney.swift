import XCTest

enum TalkItOverJourney {
    static let id = "chat/talk-it-over"
    static let version = 1
    static let emptyCopy = "No messages yet. Say hi to your cabal or float a stock idea before someone proposes a buy."

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA \(run)" }
    static func gm(run: String) -> String { "gm \(run)" }
    static func typo(run: String) -> String { "typo \(run)" }

    enum Entry {
        case search
        case card
        case here
    }

    static func openChat(_ app: XCUIApplication, run: String, step: String, from entry: Entry = .card) {
        switch entry {
        case .search:
            JoinJourney.openBySearch(app, cabal(run: run), step: step)
        case .card:
            reachCard(app, run: run, step: step).tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: cabal(run: run), timeout: 15),
                "\(step): the cabal screen for \(cabal(run: run)) did not show within 15 s"
            )
        case .here:
            break
        }
        let chat = app.element("cabal-action-chat")
        app.scrollIntoReach(chat)
        XCTAssertTrue(chat.waitForExistence(timeout: 10), "\(step): no Chat action on the cabal screen")
        chat.tap()
        XCTAssertTrue(
            JoinJourney.waitForLabel(app.element("chat-title"), containing: cabal(run: run), timeout: 15),
            "\(step): the chat for \(cabal(run: run)) did not show within 15 s"
        )
    }

    static func message(_ app: XCUIApplication, containing text: String) -> XCUIElement {
        app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'chat-message-' AND label CONTAINS %@", text)
        ).firstMatch
    }

    static func leaveScreens(_ app: XCUIApplication) {
        guard app.state == .runningForeground else { return }
        app.popToRoot()
        guard app.keyboards.firstMatch.exists else { return }
        for key in ["Return", "return", "Search", "Done"] {
            let button = app.keyboards.buttons[key].firstMatch
            if button.exists {
                button.tap()
                break
            }
        }
        if app.keyboards.firstMatch.exists {
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.12)).tap()
        }
        _ = app.keyboards.firstMatch.waitForNonExistence(timeout: 3)
    }

    static func wait(for element: XCUIElement, timeout: TimeInterval, _ failure: String) {
        XCTAssertTrue(element.waitForExistence(timeout: timeout), failure)
    }

    static func send(_ app: XCUIApplication, _ text: String, step: String) {
        let composer = app.element("chat-composer")
        XCTAssertTrue(composer.waitForExistence(timeout: 5), "\(step): no chat composer")
        composer.tap()
        composer.typeText(text)
        let send = app.element("chat-send")
        XCTAssertTrue(send.isEnabled, "\(step): the send disc is disabled with \(text) typed")
        send.tap()
    }

    static func openMenu(_ app: XCUIApplication, on text: String, step: String) {
        let bubble = message(app, containing: text)
        XCTAssertTrue(bubble.waitForExistence(timeout: 10), "\(step): no message reading \(text)")
        bubble.press(forDuration: 1.0)
        XCTAssertTrue(app.buttons["Reply"].firstMatch.waitForExistence(timeout: 5), "\(step): no Reply in the menu")
    }

    static func card(_ app: XCUIApplication, run: String) -> XCUIElement {
        app.buttons.matching(
            NSPredicate(format: "identifier BEGINSWITH 'cabals-list-card-' AND label CONTAINS %@", cabal(run: run))
        ).firstMatch
    }

    static func clearSearch(_ app: XCUIApplication) {
        let field = JoinJourney.searchField(app)
        guard field.waitForExistence(timeout: 5), let text = field.value as? String, !text.isEmpty,
            text != field.placeholderValue
        else { return }
        field.coordinate(withNormalizedOffset: CGVector(dx: 0.97, dy: 0.5)).tap()
        let left = field.value as? String ?? ""
        if !left.isEmpty, left != field.placeholderValue {
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: left.count))
        }
        leaveScreens(app)
    }

    static func reachCard(_ app: XCUIApplication, run: String, step: String) -> XCUIElement {
        app.tab("Cabals").tap()
        app.popToRoot()
        clearSearch(app)
        let found = card(app, run: run)
        let list = app.element("cabals-list")
        _ = list.waitForExistence(timeout: 10)
        var swipes = 0
        while !found.waitForExistence(timeout: 3) && swipes < 6 {
            if list.exists { list.swipeLeft() }
            swipes += 1
        }
        if !found.exists { print("JOURNEYDEBUG\n\(app.debugDescription)") }
        XCTAssertTrue(found.exists, "\(step): no card for \(cabal(run: run)) on the Cabals tab")
        return found
    }

    static func anEmptyChat(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the empty chat") {
            openChat(app, run: run, step: "S1.1", from: .search)
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("chat-empty"), containing: emptyCopy, timeout: 10),
                "S1.1: no 'No messages yet' empty state within 10 s"
            )
            let composer = app.element("chat-composer")
            XCTAssertTrue(composer.waitForExistence(timeout: 5), "S1.1: no chat composer")
            XCTAssertEqual(
                composer.placeholderValue, "Message your cabal", "S1.1: the composer placeholder is not the spec's")
            XCTAssertFalse(app.element("chat-send").isEnabled, "S1.1: the send disc is enabled on an empty draft")
        }
    }

    static func aSendsGm(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "A sends gm") {
            openChat(app, run: run, step: "S2.1")
            send(app, gm(run: run), step: "S2.1")
            let sent = message(app, containing: gm(run: run))
            wait(for: sent, timeout: 5, "S2.1: no message reading \(gm(run: run)) within 5 s")
            XCTAssertGreaterThan(sent.frame.midX, app.frame.midX, "S2.1: A's own message is not on the right")
        }
    }

    static func bReceivesGm(_ app: XCUIApplication, run: String, sender: String, recorder: JourneyRecorder) {
        recorder.step("S2.2", "B sees gm under A's name") {
            openChat(app, run: run, step: "S2.2")
            let received = message(app, containing: gm(run: run))
            wait(for: received, timeout: 15, "S2.2: no message reading \(gm(run: run)) within 15 s")
            XCTAssertLessThan(received.frame.midX, app.frame.midX, "S2.2: A's message is not on the left")
            let author = app.buttons.matching(
                NSPredicate(format: "identifier BEGINSWITH 'chat-author-' AND label CONTAINS %@", sender)
            ).firstMatch
            XCTAssertTrue(author.exists, "S2.2: no author name \(sender) over the message")
        }
    }

    static func bRepliesInThread(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.1", "B opens a thread on gm") {
            openChat(app, run: run, step: "S3.1")
            openMenu(app, on: gm(run: run), step: "S3.1")
            XCTAssertFalse(app.buttons["Delete"].firstMatch.exists, "S3.1: B can delete A's message")
            app.buttons["Reply"].firstMatch.tap()
            wait(for: app.element("chat-thread-screen"), timeout: 10, "S3.1: no thread screen within 10 s")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("chat-thread-parent-body"), containing: gm(run: run), timeout: 10),
                "S3.1: the thread's parent does not read \(gm(run: run))"
            )
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("chat-thread-empty"), containing: "No replies yet. Start the thread.", timeout: 10),
                "S3.1: no 'No replies yet' in the thread"
            )
        }

        recorder.step("S3.2", "B replies gm back") {
            let toggle = app.element("chat-thread-also-in-channel")
            XCTAssertTrue(toggle.waitForExistence(timeout: 5), "S3.2: no 'Also send to channel' toggle")
            XCTAssertEqual(toggle.value as? String, "0", "S3.2: 'Also send to channel' starts on")
            let composer = app.element("chat-composer")
            XCTAssertEqual(
                composer.placeholderValue, "Reply in thread", "S3.2: the thread composer placeholder is not the spec's")
            send(app, "gm back", step: "S3.2")
            wait(for: message(app, containing: "gm back"), timeout: 5, "S3.2: no reply reading gm back within 5 s")
        }
    }

    static func aSeesReply(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.3", "A sees the reply count and Seen by 1") {
            openChat(app, run: run, step: "S3.3")
            let replies = app.descendants(matching: .any).matching(
                NSPredicate(format: "identifier BEGINSWITH 'chat-replies-' AND label CONTAINS '1 reply · last reply'")
            ).firstMatch
            wait(for: replies, timeout: 15, "S3.3: no '1 reply · last reply' under \(gm(run: run)) within 15 s")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("chat-seen-label"), containing: "Seen by 1", timeout: 15),
                "S3.3: no 'Seen by 1' under \(gm(run: run)) within 15 s"
            )
            XCTAssertFalse(message(app, containing: "gm back").exists, "S3.3: gm back is in the channel")
        }
    }

    static func aDeletesTypo(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S4.1", "A deletes typo") {
            openChat(app, run: run, step: "S4.1")
            send(app, typo(run: run), step: "S4.1")
            openMenu(app, on: typo(run: run), step: "S4.1")
            let delete = app.buttons["Delete"].firstMatch
            XCTAssertTrue(delete.waitForExistence(timeout: 5), "S4.1: no Delete in the menu")
            delete.tap()
            wait(for: app.staticTexts["Delete this message?"], timeout: 5, "S4.1: no 'Delete this message?' dialog")
            XCTAssertTrue(
                app.staticTexts["It's removed for everyone in the cabal."].exists,
                "S4.1: the dialog has no removal copy"
            )
            app.buttons["Delete"].firstMatch.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("monaco-toast-banner"), containing: "Message deleted", timeout: 10),
                "S4.1: no 'Message deleted' toast within 10 s"
            )
            XCTAssertTrue(
                message(app, containing: typo(run: run)).waitForNonExistence(timeout: 10),
                "S4.1: \(typo(run: run)) is still on screen"
            )
        }
    }
}

extension TalkItOverJourney {
    static func aSendsTwo(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S5.1", "A sends one and two") {
            openChat(app, run: run, step: "S5.1")
            for text in ["one \(run)", "two \(run)"] {
                send(app, text, step: "S5.1")
                wait(for: message(app, containing: text), timeout: 5, "S5.1: no message reading \(text) within 5 s")
            }
        }
    }

    static func bSeesBadge(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S5.2", "B sees 2 unread on the card") {
            let found = reachCard(app, run: run, step: "S5.2")
            let deadline = Date().addingTimeInterval(10)
            while !found.label.contains("2 unread messages") && Date() < deadline {
                RunLoop.current.run(until: Date().addingTimeInterval(0.3))
            }
            XCTAssertTrue(
                found.label.contains("2 unread messages"),
                "S5.2: the card reads '\(found.label)', not '2 unread messages'"
            )
        }
    }

    static func bOpensUnreadChat(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S5.3", "B opens the unread chat") {
            card(app, run: run).tap()
            let chat = app.element("cabal-action-chat")
            app.scrollIntoReach(chat)
            XCTAssertTrue(chat.waitForExistence(timeout: 15), "S5.3: no Chat action on the cabal screen")
            XCTAssertEqual(chat.label, "Chat, unread messages", "S5.3: Chat shows no unread dot")
            openChat(app, run: run, step: "S5.3", from: .here)
            wait(
                for: message(app, containing: "two \(run)"), timeout: 10,
                "S5.3: no message reading two \(run) within 10 s")
            XCTAssertTrue(message(app, containing: gm(run: run)).exists, "S5.3: no message reading \(gm(run: run))")
            XCTAssertFalse(
                message(app, containing: typo(run: run)).exists, "S5.3: \(typo(run: run)) is still in B's chat")
        }
    }

    static func bClearsUnread(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S5.4", "the unread dot and badge clear") {
            app.tapBack()
            let chat = app.element("cabal-action-chat")
            XCTAssertTrue(chat.waitForExistence(timeout: 10), "S5.4: no Chat action after backing out")
            let deadline = Date().addingTimeInterval(10)
            while chat.label != "Chat" && Date() < deadline {
                RunLoop.current.run(until: Date().addingTimeInterval(0.3))
            }
            XCTAssertEqual(chat.label, "Chat", "S5.4: Chat still shows an unread dot")
            app.tapBack()
            let found = reachCard(app, run: run, step: "S5.4")
            XCTAssertFalse(found.label.contains("unread"), "S5.4: the card still shows unread: \(found.label)")
        }
    }
}
