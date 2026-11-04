import XCTest

struct JourneyStep {
    let id: String
    let name: String
    let actor: String
    let seconds: TimeInterval
    let copy: [String]

    func filled(_ index: Int, _ values: [String: String]) -> String {
        values.reduce(copy[index]) { text, pair in
            text.replacingOccurrences(of: "{\(pair.key)}", with: pair.value)
        }
    }
}

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

enum JourneyWait: Equatable {
    case found
    case deadEnd(id: String, label: String?, after: TimeInterval)
    case timedOut(nearest: [String], title: String?)
}

enum JourneyDiagnostics {
    static let pollInterval: TimeInterval = 0.2
    static let alertInterval: TimeInterval = 1

    @discardableResult
    static func wait(
        for element: XCUIElement, in app: XCUIApplication, timeout: TimeInterval,
        step: String = "journey", allowing: [String] = [], failOnMiss: Bool = true
    ) -> Bool {
        let result = waitResult(for: element, in: app, timeout: timeout, allowing: allowing)
        if result == .found { return true }
        let message = message(for: result, step: step, target: targetName(element), timeout: timeout)
        print(message)
        attach(app)
        XCTContext.runActivity(named: message) { _ in }
        if failOnMiss { XCTFail(message) }
        return false
    }

    static func waitResult(
        for element: XCUIElement, in app: XCUIApplication, timeout: TimeInterval, allowing: [String] = []
    ) -> JourneyWait {
        let start = Date()
        let deadline = start.addingTimeInterval(timeout)
        let alert = XCUIApplication(bundleIdentifier: "com.apple.springboard").alerts.firstMatch
        var lastAlertCheck = Date.distantPast
        var seen: [String] = []
        var title: String?
        repeat {
            if element.exists { return .found }
            if let snapshot = try? app.snapshot() {
                var ids: [String] = []
                var labels: [String: String] = [:]
                var bar: String?
                collect(snapshot, ids: &ids, labels: &labels, bar: &bar)
                seen = ids
                title = bar
                if let id = deadEndID(in: ids, allowing: allowing) {
                    return .deadEnd(id: id, label: labels[id], after: Date().timeIntervalSince(start))
                }
            }
            if Date().timeIntervalSince(lastAlertCheck) >= alertInterval {
                lastAlertCheck = Date()
                if alert.exists, !allowing.contains("system-alert") {
                    return .deadEnd(id: "system-alert", label: alert.label, after: Date().timeIntervalSince(start))
                }
            }
            RunLoop.current.run(until: Date().addingTimeInterval(pollInterval))
        } while Date() < deadline
        if element.exists { return .found }
        return .timedOut(nearest: Array(seen), title: title)
    }

    private static func collect(
        _ snapshot: XCUIElementSnapshot, ids: inout [String], labels: inout [String: String], bar: inout String?
    ) {
        if !snapshot.identifier.isEmpty {
            ids.append(snapshot.identifier)
            if !snapshot.label.isEmpty { labels[snapshot.identifier] = snapshot.label }
        }
        if bar == nil, snapshot.elementType == .navigationBar {
            bar = snapshot.identifier.isEmpty ? (snapshot.label.isEmpty ? nil : snapshot.label) : snapshot.identifier
        }
        for child in snapshot.children {
            collect(child, ids: &ids, labels: &labels, bar: &bar)
        }
    }

    static func targetName(_ element: XCUIElement) -> String {
        element.identifier.isEmpty ? "element \(element.debugDescription.prefix(60))" : element.identifier
    }

    static func deadEndID(in ids: [String], allowing: [String]) -> String? {
        ids.first { id in
            (id.hasSuffix("-failed") || id.hasSuffix("-error")) && !allowing.contains(id)
        }
    }

    static func levenshtein(_ a: String, _ b: String) -> Int {
        let a = Array(a)
        let b = Array(b)
        if a.isEmpty { return b.count }
        if b.isEmpty { return a.count }
        var previous = Array(0...b.count)
        for i in 1...a.count {
            var row = [i]
            for j in 1...b.count {
                row.append(min(previous[j] + 1, row[j - 1] + 1, previous[j - 1] + (a[i - 1] == b[j - 1] ? 0 : 1)))
            }
            previous = row
        }
        return previous[b.count]
    }

    static func nearest(_ ids: [String], to target: String, limit: Int = 10) -> [String] {
        var unique: [String] = []
        for id in ids where !unique.contains(id) { unique.append(id) }
        let ranked = unique.enumerated().sorted {
            let l = levenshtein($0.element, target)
            let r = levenshtein($1.element, target)
            return l == r ? $0.offset < $1.offset : l < r
        }
        return ranked.prefix(limit).map(\.element)
    }

    static func message(for result: JourneyWait, step: String, target: String, timeout: TimeInterval) -> String {
        switch result {
        case .found:
            return "\(step): \(target) showed"
        case .deadEnd(let id, let label, let after):
            let shown = label.map { " (\"\($0)\")" } ?? ""
            return
                "\(step): waited for \(target) but the screen shows \(id)\(shown) after \(String(format: "%.1f", after)) s"
        case .timedOut(let ids, let title):
            let near = nearest(ids, to: target).joined(separator: ", ")
            let name = title.map { " (title \"\($0)\")" } ?? ""
            return "\(step): \(target) did not show within \(Int(timeout)) s; on screen: \(near) …\(name)"
        }
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

final class JourneyRecorder {
    let journey: String
    let version: Int
    private let clock = ContinuousClock()

    init(journey: String, version: Int) {
        self.journey = journey
        self.version = version
    }

    func step(_ step: JourneyStep, _ body: () throws -> Void) rethrows {
        try self.step(step.id, step.name, body)
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
    func act(as actor: String, viaUI: Bool = false) throws -> JourneyAccount {
        let account = try account(actor)
        let running = app.state == .runningForeground
        if self.actor == actor, running {
            let screen = SignInJourney.currentScreen(app, timeout: 2)
            if screen == .tabs || (screen == nil && app.navigationBars.buttons["BackButton"].firstMatch.exists) {
                return account
            }
        }
        SignInJourney.switchActor(app, to: account, viaUI: viaUI)
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

    func revealTabBar(maxBacks: Int = 4) {
        for _ in 0..<maxBacks {
            guard !tabBars.firstMatch.exists else { return }
            let back = navigationBars.buttons["BackButton"].firstMatch
            guard back.exists else { return }
            back.tap()
            _ = tabBars.firstMatch.waitForExistence(timeout: 2)
        }
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
        guard element("push-pre-prompt").waitForExistence(timeout: timeout) else { return }
        answerVotePermissionSheet(turnOn: false, timeout: timeout)
    }

    func holdToCommit(_ element: XCUIElement, duration: TimeInterval = 0.9) {
        element.press(forDuration: duration)
    }

    func pickAndHold(_ choice: String, timeout: TimeInterval = 10, step: String = "journey") {
        let button = element("proposal-vote-\(choice)")
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: button, in: self, timeout: timeout, step: step),
            "proposal-vote-\(choice) did not show within \(Int(timeout)) s")
        button.tap()
        holdToCommit(button)
    }

    func changeVote(to choice: String, timeout: TimeInterval = 10, step: String = "journey") {
        let change = element("proposal-change-vote")
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: change, in: self, timeout: timeout, step: step),
            "proposal-change-vote did not show within \(Int(timeout)) s")
        change.tap()
        pickAndHold(choice, timeout: timeout, step: step)
    }

    func homeVoteCardPickAndHold(
        proposalID: String, choice: String, timeout: TimeInterval = 10, step: String = "journey"
    ) {
        let button = element("home-vote-\(choice)-\(proposalID)")
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: button, in: self, timeout: timeout, step: step),
            "home-vote-\(choice)-\(proposalID) did not show within \(Int(timeout)) s")
        button.tap()
        holdToCommit(button)
    }

    func toastText(timeout: TimeInterval = 10, step: String = "journey") -> String? {
        let banner = element("monaco-toast-banner")
        guard JourneyDiagnostics.wait(for: banner, in: self, timeout: timeout, step: step, failOnMiss: false) else {
            return nil
        }
        return banner.label
    }

    @discardableResult
    func tapToastAction(timeout: TimeInterval = 10, step: String = "journey") -> Bool {
        let action = element("monaco-toast-action")
        guard JourneyDiagnostics.wait(for: action, in: self, timeout: timeout, step: step, failOnMiss: false) else {
            return false
        }
        action.tap()
        return true
    }

    @discardableResult
    func answerVotePermissionSheet(turnOn: Bool, timeout: TimeInterval = 10, step: String = "journey") -> Bool {
        let sheet = element("push-pre-prompt")
        guard JourneyDiagnostics.wait(for: sheet, in: self, timeout: timeout, step: step, failOnMiss: false) else {
            return false
        }
        buttons[turnOn ? "push-pre-prompt-turn-on" : "push-pre-prompt-not-now"].firstMatch.tap()
        return sheet.waitForNonExistence(timeout: 5)
    }

    func openCabal(named name: String, tab: String = "treasury", timeout: TimeInterval = 15, step: String = "journey") {
        self.tab("Cabals").tap()
        let tile = buttons.matching(
            NSPredicate(format: "identifier BEGINSWITH 'cabals-tile-' AND label CONTAINS %@", name)
        ).firstMatch
        _ = element("cabals-grid").waitForExistence(timeout: 10)
        if revealInGrid(tile) {
            tile.tap()
        } else {
            JoinJourney.openBySearch(self, name, step: "openCabal")
        }
        let titled = ["cabal-header-name", "cabal-inline-title"].map { id in
            descendants(matching: .any).matching(
                NSPredicate(format: "identifier == %@ AND label CONTAINS %@", id, name)
            ).firstMatch
        }
        guard waitForFirst(of: titled, timeout: timeout) != nil else {
            XCTFail("\(step): cabal-header-name did not read \(name) on the \(tab) tab within \(Int(timeout)) s")
            return
        }
        if element("cabal-join").exists { return }
        if tab.lowercased() == "chat" {
            openCabalTab("chat", step: step)
            XCTAssertTrue(
                element("chat-view").waitForExistence(timeout: timeout),
                "\(step): chat-view did not show on the chat tab within \(Int(timeout)) s")
            return
        }
        openCabalTab(tab, step: step)
        let header = element("cabal-header-name")
        let shown = NSPredicate(format: "label CONTAINS %@", name)
        let ready = XCTNSPredicateExpectation(predicate: shown, object: header)
        XCTAssertEqual(
            XCTWaiter().wait(for: [ready], timeout: timeout), .completed,
            "\(step): cabal-header-name did not read \(name) on the \(tab) tab within \(Int(timeout)) s")
    }

    func isAboveTabBar(_ element: XCUIElement) -> Bool {
        let bar = tabBars.firstMatch
        let top = bar.exists && !bar.frame.isEmpty ? bar.frame.minY : windows.firstMatch.frame.height * 0.85
        return element.frame.maxY <= top
    }

    func revealInGrid(_ tile: XCUIElement, maxSwipes: Int = 8) -> Bool {
        var swipes = 0
        while !(tile.exists && tile.isHittable && isAboveTabBar(tile)) && swipes < maxSwipes {
            swipeUp()
            swipes += 1
        }
        return tile.exists && tile.isHittable && isAboveTabBar(tile)
    }

    func openCabalTab(_ name: String, timeout: TimeInterval = 10, step: String = "journey") {
        let plain = element("cabal-tab-\(name.lowercased())")
        let candidates = [plain, element("cabal-tab-pinned-\(name.lowercased())")]
        var found = waitForFirst(of: candidates, timeout: 2)
        var swipes = 0
        while found == nil && swipes < 4 {
            swipeDown()
            swipes += 1
            found = waitForFirst(of: candidates, timeout: 2)
        }
        let tab = found.map { candidates[$0] } ?? plain
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: tab, in: self, timeout: timeout, step: step),
            "cabal-tab-\(name.lowercased()) did not show within \(Int(timeout)) s")
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
            let system = navigationBars.buttons["BackButton"].firstMatch
            let custom = navigationBars.buttons["Back"].firstMatch
            guard let index = waitForFirst(of: [system, custom], timeout: 1) else { return }
            let back = [system, custom][index]
            guard back.isHittable else { return }
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

extension XCUIElement {
    func tapSwitch() {
        coordinate(withNormalizedOffset: CGVector(dx: value as? String == "1" ? 0.08 : 0.92, dy: 0.5)).tap()
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
