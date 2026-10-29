import XCTest

struct JourneyAccount {
    enum Channel: String {
        case sms
        case email
    }

    let actor: String
    let phone: String
    let email: String
    let code: String
    let name: String
    let channel: Channel

    var address: String {
        channel == .sms ? phone : email
    }

    static func load(
        actor: String = ProcessInfo.processInfo.environment["MONACO_QA_ACTOR"] ?? "A",
        channel: Channel? = nil,
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) throws -> JourneyAccount {
        guard environment["MONACO_QA_JOURNEYS"] == "1" else {
            throw XCTSkip("live journey tests run through scripts/qa/journey.py, which sets MONACO_QA_JOURNEYS=1")
        }
        let prefix = "MONACO_QA_\(actor)_"
        let phone = environment[prefix + "PHONE"] ?? ""
        let email = environment[prefix + "EMAIL"] ?? ""
        let code = environment[prefix + "CODE"] ?? ""
        let channel = channel ?? Channel(rawValue: environment["MONACO_QA_CHANNEL"] ?? "sms") ?? .sms
        let name = environment[prefix + "NAME"] ?? ""
        let account = JourneyAccount(
            actor: actor, phone: phone, email: email, code: code, name: name, channel: channel)
        guard !account.address.isEmpty, code.count == 6, !name.isEmpty else {
            throw XCTSkip(
                "no \(channel.rawValue) login for actor \(actor): set \(prefix)PHONE, \(prefix)EMAIL, \(prefix)CODE and \(prefix)NAME"
            )
        }
        return account
    }
}

enum JourneyRun {
    static func id(environment: [String: String] = ProcessInfo.processInfo.environment) throws -> String {
        guard let run = environment["MONACO_QA_RUN"], !run.isEmpty else {
            XCTFail("no {QA.run}: scripts/qa/journey.py sets MONACO_QA_RUN for each run")
            throw CocoaError(.keyValueValidation)
        }
        return run
    }
}

enum JourneyDiagnostics {
    static func wait(for element: XCUIElement, in app: XCUIApplication, timeout: TimeInterval) -> Bool {
        let found = element.waitForExistence(timeout: timeout)
        if !found {
            attach(app)
        }
        return found
    }

    static func attach(_ app: XCUIApplication, logLines: Int = 50) {
        XCTContext.runActivity(named: "diagnostics") { activity in
            let tree = XCTAttachment(string: app.debugDescription)
            tree.name = "app.debugDescription"
            tree.lifetime = .keepAlways
            activity.add(tree)
            guard let path = ProcessInfo.processInfo.environment["MONACO_QA_API_LOG"], !path.isEmpty,
                let log = try? String(contentsOfFile: path, encoding: .utf8)
            else { return }
            let tail = XCTAttachment(string: log.split(separator: "\n").suffix(logLines).joined(separator: "\n"))
            tail.name = "api.log (last \(logLines) lines)"
            tail.lifetime = .keepAlways
            activity.add(tail)
        }
    }
}

enum JourneyWait {
    static func until(timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if condition() { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        } while Date() < deadline
        return condition()
    }
}

final class JourneyRecorder {
    let journey: String
    let version: Int
    private let clock = ContinuousClock()

    init(journey: String, version: Int) {
        self.journey = journey
        self.version = version
    }

    func step(_ id: String, _ name: String, _ body: () throws -> Void) rethrows {
        print("JOURNEYSTEP\tbegin\t\(journey)@\(version)\t\(id)\t\(name)")
        let start = clock.now
        try XCTContext.runActivity(named: "\(journey) \(id): \(name)") { _ in
            try body()
        }
        let elapsed = clock.now - start
        let millis =
            Int(elapsed.components.seconds) * 1000 + Int(elapsed.components.attoseconds / 1_000_000_000_000_000)
        print("JOURNEYSTEP\tend\t\(journey)@\(version)\t\(id)\t\(millis)")
    }
}

final class JourneySession {
    let app = XCUIApplication.monacoForJourneys()
    private let environment: [String: String]
    private var actor: String?
    private var finished = false
    private let clock = ContinuousClock()
    private static let setupTimeout: TimeInterval = 240

    init(
        actors: [String] = ["A"],
        channel: JourneyAccount.Channel? = nil,
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) throws {
        guard let first = actors.first else {
            XCTFail("JourneySession needs the journey's declared actors")
            throw CocoaError(.keyValueValidation)
        }
        _ = try JourneyAccount.load(actor: first, channel: channel, environment: environment)
        self.environment = environment
    }

    func account(_ actor: String) throws -> JourneyAccount {
        try JourneyAccount.load(actor: actor, environment: environment)
    }

    @discardableResult
    func act(as actor: String) throws -> JourneyAccount {
        let account = try account(actor)
        let running = app.state == .runningForeground
        if self.actor == actor, running, SignInJourney.currentScreen(app, timeout: 2) == .tabs {
            return account
        }
        SignInJourney.switchActor(app, to: account)
        self.actor = actor
        return account
    }

    func scenario(_ id: String, _ body: () throws -> Void) throws {
        guard !finished else { return }
        try waitForSetup(id)
        print("JOURNEYSCENARIO\tbegin\t\(id)")
        let start = clock.now
        try body()
        let elapsed = clock.now - start
        let millis =
            Int(elapsed.components.seconds) * 1000 + Int(elapsed.components.attoseconds / 1_000_000_000_000_000)
        print("JOURNEYSCENARIO\tend\t\(id)\t\(millis)")
        finished = environment["MONACO_QA_LAST_SCENARIO"] == id
    }

    private func waitForSetup(_ id: String) throws {
        guard let path = environment["MONACO_QA_SETUP_DIR"], !path.isEmpty else { return }
        let folder = URL(fileURLWithPath: path)
        let done = folder.appendingPathComponent("\(id).done")
        try Data().write(to: folder.appendingPathComponent("\(id).request"))
        let deadline = Date().addingTimeInterval(Self.setupTimeout)
        while !FileManager.default.fileExists(atPath: done.path) {
            guard Date() < deadline else {
                XCTFail("\(id): journey.py did not finish the setup script within \(Int(Self.setupTimeout)) s")
                throw CocoaError(.fileReadNoSuchFile)
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        }
        let status = (try? String(contentsOf: done, encoding: .utf8)) ?? ""
        guard status == "ok" else {
            XCTFail("\(id): the setup script failed: \(status)")
            throw CocoaError(.keyValueValidation)
        }
    }
}

enum JourneyHandoff {
    private static func fileURL() throws -> URL {
        guard let path = ProcessInfo.processInfo.environment["MONACO_QA_HANDOFF"], !path.isEmpty else {
            throw XCTSkip("no hand-off file: a phase runs through scripts/qa/journey.py, which sets MONACO_QA_HANDOFF")
        }
        return URL(fileURLWithPath: path)
    }

    private static func load(_ url: URL) -> [String: String] {
        guard let data = try? Data(contentsOf: url) else { return [:] }
        return (try? JSONDecoder().decode([String: String].self, from: data)) ?? [:]
    }

    static func write(_ key: String, _ value: String) throws {
        let url = try fileURL()
        var values = load(url)
        values[key] = value
        try JSONEncoder().encode(values).write(to: url, options: .atomic)
    }

    static func read(_ key: String) throws -> String {
        let url = try fileURL()
        guard let value = load(url)[key] else {
            XCTFail("hand-off has no '\(key)': the setup script that writes it did not run or did not reach it")
            throw CocoaError(.fileReadNoSuchFile)
        }
        return value
    }
}

extension XCUIApplication {
    static func monacoForJourneys() -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoFeatureConnectX", "YES"]
        if let baseURL = ProcessInfo.processInfo.environment["MONACO_QA_API_BASE_URL"], !baseURL.isEmpty {
            app.launchEnvironment["MONACO_API_BASE_URL"] = baseURL
        }
        return app
    }

    func element(_ identifier: String) -> XCUIElement {
        descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    func tab(_ title: String) -> XCUIElement {
        tabBars.buttons[title]
    }

    func waitForFirst(of elements: [XCUIElement], timeout: TimeInterval) -> Int? {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if let index = elements.firstIndex(where: { $0.exists }) {
                return index
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        } while Date() < deadline
        return nil
    }

    func dismissConfirmDialog(title: String, timeout: TimeInterval = 5) {
        tapOutsidePopover()
        _ = staticTexts[title].firstMatch.waitForNonExistence(timeout: timeout)
    }

    func confirmDialogButton(_ identifier: String) -> XCUIElement {
        buttons[identifier].firstMatch
    }

    @discardableResult
    func answerSystemAlert(allow: Bool, timeout: TimeInterval = 15) -> Bool {
        let alert = XCUIApplication(bundleIdentifier: "com.apple.springboard").alerts.firstMatch
        guard alert.waitForExistence(timeout: timeout) else { return false }
        let button = alert.buttons[allow ? "Allow" : "Don’t Allow"].firstMatch
        if button.exists {
            button.tap()
        } else {
            alert.buttons[allow ? "Allow" : "Don't Allow"].firstMatch.tap()
        }
        return alert.waitForNonExistence(timeout: 5)
    }

    func waitForToastGone(timeout: TimeInterval = 6) {
        _ = element("monaco-toast-banner").waitForNonExistence(timeout: timeout)
    }

    func dismissPushPrePromptIfShown(timeout: TimeInterval = 3) {
        answerVotePermissionSheet(turnOn: false, timeout: timeout)
    }

    @discardableResult
    func answerVotePermissionSheet(turnOn: Bool, timeout: TimeInterval = 10) -> Bool {
        let sheet = element("push-pre-prompt")
        guard sheet.waitForExistence(timeout: timeout) else { return false }
        let answer = buttons[turnOn ? "push-pre-prompt-turn-on" : "push-pre-prompt-not-now"].firstMatch
        guard answer.waitForExistence(timeout: 5) else { return false }
        answer.tap()
        if turnOn { _ = answerSystemAlert(allow: true, timeout: 5) }
        return element("push-pre-prompt").waitForNonExistence(timeout: 5)
    }

    func holdToCommit(_ target: XCUIElement, duration: TimeInterval = 0.9) {
        target.press(forDuration: duration)
    }

    func pickAndHold(_ choice: String) {
        let target = element("proposal-vote-\(choice)")
        XCTAssertTrue(target.waitForExistence(timeout: 10), "proposal-vote-\(choice) did not show within 10 s")
        target.tap()
        holdToCommit(target)
    }

    func changeVote(to choice: String) {
        let change = element("proposal-change-vote")
        XCTAssertTrue(change.waitForExistence(timeout: 10), "proposal-change-vote did not show within 10 s")
        change.tap()
        pickAndHold(choice)
    }

    func homeVoteCardPickAndHold(proposalID: String, choice: String) {
        let target = element("home-vote-\(choice)-\(proposalID)")
        XCTAssertTrue(
            target.waitForExistence(timeout: 10), "home-vote-\(choice)-\(proposalID) did not show within 10 s")
        target.tap()
        holdToCommit(target)
    }

    func toastText(timeout: TimeInterval = 10) -> String? {
        let banner = element("monaco-toast-banner")
        guard banner.waitForExistence(timeout: timeout) else { return nil }
        if !banner.label.isEmpty { return banner.label }
        return banner.staticTexts.firstMatch.label
    }

    @discardableResult
    func tapToastAction(timeout: TimeInterval = 10) -> Bool {
        let action = element("monaco-toast-action")
        guard action.waitForExistence(timeout: timeout) else { return false }
        action.tap()
        return true
    }

    func openCabal(named name: String, timeout: TimeInterval = 15) {
        tab("Cabals").tap()
        let tile = descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'cabals-tile-' AND label BEGINSWITH %@", name)
        ).firstMatch
        if !tile.waitForExistence(timeout: 5) {
            let plus = element("cabals-new-button")
            XCTAssertTrue(
                plus.waitForExistence(timeout: timeout), "cabals-new-button did not show within \(Int(timeout)) s")
            plus.tap()
            let find = buttons["cabals-menu-find"].firstMatch
            XCTAssertTrue(find.waitForExistence(timeout: 5), "cabals-menu-find did not show within 5 s")
            find.tap()
            let field = textFields["cabals-search-field"].firstMatch
            XCTAssertTrue(
                field.waitForExistence(timeout: timeout), "cabals-search-field did not show within \(Int(timeout)) s")
            field.tap()
            field.typeText(name)
            let result = descendants(matching: .any).matching(
                NSPredicate(format: "identifier BEGINSWITH 'cabals-search-result-'")
            ).firstMatch
            XCTAssertTrue(
                result.waitForExistence(timeout: timeout),
                "no cabals-search-result for \(name) within \(Int(timeout)) s")
            dismissKeyboard()
            result.tap()
        } else {
            tile.tap()
        }
        let header = element("cabal-header-name")
        XCTAssertTrue(
            header.waitForExistence(timeout: timeout), "cabal-header-name did not show within \(Int(timeout)) s")
        XCTAssertTrue(
            JourneyWait.until(timeout: timeout) { header.label == name },
            "cabal-header-name reads \(header.label), not \(name)")
    }

    func openCabalTab(_ name: String) {
        let tab = element("cabal-tab-\(name.lowercased())")
        scrollIntoReach(tab)
        XCTAssertTrue(tab.waitForExistence(timeout: 10), "cabal-tab-\(name.lowercased()) did not show within 10 s")
        tab.tap()
    }

    func tapBack(timeout: TimeInterval = 5) {
        let back = navigationBars.buttons["BackButton"].firstMatch
        let hittable = NSPredicate(format: "exists == true AND hittable == true")
        let ready = XCTNSPredicateExpectation(predicate: hittable, object: back)
        XCTAssertEqual(XCTWaiter().wait(for: [ready], timeout: timeout), .completed, "Back was not tappable")
        back.tap()
    }

    func dismissKeyboard() {
        guard keyboards.firstMatch.exists else { return }
        for key in ["Return", "return", "Search", "Done"] {
            let button = keyboards.buttons[key].firstMatch
            if button.exists {
                button.tap()
                break
            }
        }
        if keyboards.firstMatch.exists {
            coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.12)).tap()
        }
        _ = keyboards.firstMatch.waitForNonExistence(timeout: 3)
    }

    func popToRoot(maxDepth: Int = 8) {
        for _ in 0..<maxDepth {
            let back = navigationBars.buttons["BackButton"].firstMatch
            guard back.waitForExistence(timeout: 1), back.isHittable else { return }
            back.tap()
        }
    }

    func shareSheetShows(timeout: TimeInterval = 5) -> Bool {
        waitForFirst(of: [buttons["Close"].firstMatch, otherElements["ActivityListView"].firstMatch], timeout: timeout)
            != nil
    }

    @discardableResult
    func dismissShareSheet(timeout: TimeInterval = 5) -> Bool {
        guard shareSheetShows(timeout: timeout) else { return false }
        let close = buttons["Close"].firstMatch
        if close.waitForExistence(timeout: 1) {
            close.tap()
        } else {
            tapOutsidePopover()
        }
        return true
    }

    private func tapOutsidePopover() {
        let region = otherElements["PopoverDismissRegion"].firstMatch
        if region.exists {
            region.tap()
        } else {
            coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.05)).tap()
        }
    }

    func scrollIntoReach(_ element: XCUIElement, maxSwipes: Int = 8) {
        var swipes = 0
        while !element.isHittable && swipes < maxSwipes {
            swipeUp()
            swipes += 1
        }
    }
}

extension XCTestCase {
    func attachScreenshot(of app: XCUIApplication, named name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
