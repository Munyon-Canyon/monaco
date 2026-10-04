import Foundation
import MonacoAPI
import Observation

public enum LinkStepCaption: Equatable, Sendable {
    case error(String)
    case note(String)
}

public enum LinkStepResult: Equatable, Sendable {
    case stay
    case toast(String)
    case finished(SessionProfile)
}

public enum LinkCopy {
    public static let phoneTitle = "Add your number"
    public static let phoneSubtext = "We match your contacts to find friends. Your number stays private."
    public static let sendCode = "Send code"
    public static let sendingCode = "Sending code\u{2026}"
    public static let linking = "Linking\u{2026}"
    public static let changeNumber = "Change number"
    public static let newCodeSent = "New code sent."
    public static let phoneLinkedElsewhere = "This number is linked to another account."
    public static let invalidCode = "That code didn't work. Check it and try again."
    public static let phoneAdded = "Number added."

    public static let xTitle = "Connect X"
    public static let xSubtext = "Find people you follow on Monaco."
    public static let connectX = "Connect X"
    public static let connecting = "Connecting\u{2026}"
    public static let xLinkedElsewhere = "This X account is linked to another account."
    public static let xConnected = "X connected."

    public static let skip = "Skip"
    public static let notNow = "Not now"
    public static let unknown = "Something went wrong. Try again."

    public static func codeSent(to number: String) -> String { "Code sent to \(number)" }

    public static let auditedStrings = [
        phoneTitle, phoneSubtext, sendCode, sendingCode, linking, changeNumber, newCodeSent,
        phoneLinkedElsewhere, invalidCode, phoneAdded, xTitle, xSubtext, connectX, connecting, xLinkedElsewhere,
        xConnected, skip, notNow, unknown, ResendCooldown.readyLabel,
    ]

    static func caption(for error: LinkError, linkedElsewhere: String) -> LinkStepCaption? {
        switch error {
        case .alreadyLinkedElsewhere: .error(linkedElsewhere)
        case .invalidCode: .error(invalidCode)
        case .cancelled: nil
        case .network: .error(ToastCopy.message(for: .transport(URLError(.notConnectedToInternet))))
        case .unknown: .error(unknown)
        }
    }

    static func result(for error: any Error) -> (LinkStepResult, LinkStepCaption?) {
        let apiError = APIError(error)
        if case .problem(let problem) = apiError, notLinkedCodes.contains(problem.code.wire) {
            return (.toast(problem.message), nil)
        }
        return (.stay, .error(ToastCopy.message(for: apiError)))
    }

    private static let notLinkedCodes: Set<String> = ["phone_not_linked", "x_not_linked"]
}

@Observable
@MainActor
public final class PhoneLinkModel {
    public enum Step: Equatable, Sendable {
        case number
        case code(sentTo: String)
    }

    public enum Activity: Equatable, Sendable {
        case idle
        case sending
        case linking
        case skipping
    }

    public static let codeLength = 6

    public private(set) var step = Step.number
    public private(set) var activity = Activity.idle
    public private(set) var caption: LinkStepCaption?
    public private(set) var linkedElsewhere = false
    public private(set) var cooldown: ResendCooldown
    public var code = "" {
        didSet { if code != oldValue, !code.isEmpty { caption = nil } }
    }

    private let linking: any AccountLinking
    private let onboarding: OnboardingAPI
    private var linkedUpstream = false
    private var storeSubmission = IdempotentSubmission()
    private let skipSubmission = IdempotentSubmission()

    public init(linking: any AccountLinking, onboarding: OnboardingAPI, clock: some Clock<Duration>) {
        self.linking = linking
        self.onboarding = onboarding
        cooldown = ResendCooldown(clock: clock)
    }

    public var isBusy: Bool { activity != .idle }

    public var canLink: Bool {
        guard case .code = step, !isBusy else { return false }
        return code.count == Self.codeLength
    }

    public func sendCode(to e164: String) async {
        guard !isBusy else { return }
        activity = .sending
        defer { activity = .idle }
        caption = nil
        linkedElsewhere = false
        do {
            try await linking.sendPhoneCode(e164: e164)
        } catch {
            fail(error)
            return
        }
        code = ""
        linkedUpstream = false
        storeSubmission = IdempotentSubmission()
        cooldown.restart()
        step = .code(sentTo: e164)
    }

    public func resend() async {
        guard case .code(let sentTo) = step, cooldown.canResend, !isBusy else { return }
        activity = .sending
        defer { activity = .idle }
        caption = nil
        do {
            try await linking.sendPhoneCode(e164: sentTo)
        } catch {
            fail(error)
            return
        }
        code = ""
        cooldown.restart()
        caption = .note(LinkCopy.newCodeSent)
    }

    public func changeNumber() {
        guard !isBusy else { return }
        code = ""
        caption = nil
        linkedElsewhere = false
        step = .number
    }

    public func link() async -> LinkStepResult {
        guard canLink else { return .stay }
        activity = .linking
        defer { activity = .idle }
        caption = nil
        if !linkedUpstream {
            do {
                try await linking.linkPhone(code: code)
            } catch {
                fail(error)
                return .stay
            }
            linkedUpstream = true
        }
        return await store { [onboarding, storeSubmission] in
            try await onboarding.linkPhone(submission: storeSubmission)
        }
    }

    public func skip() async -> LinkStepResult {
        guard !isBusy else { return .stay }
        activity = .skipping
        defer { activity = .idle }
        caption = nil
        return await store { [onboarding, skipSubmission] in
            try await onboarding.skip(.phone, submission: skipSubmission)
        }
    }

    private func store(_ call: () async throws -> SessionProfile) async -> LinkStepResult {
        do {
            return .finished(try await call())
        } catch {
            let (result, caption) = LinkCopy.result(for: error)
            self.caption = caption
            return result
        }
    }

    private func fail(_ error: any Error) {
        let linkError = error as? LinkError ?? .unknown
        linkedElsewhere = linkError == .alreadyLinkedElsewhere
        caption = LinkCopy.caption(for: linkError, linkedElsewhere: LinkCopy.phoneLinkedElsewhere)
    }
}

@Observable
@MainActor
public final class XLinkModel {
    public enum Activity: Equatable, Sendable {
        case idle
        case connecting
        case skipping
    }

    public private(set) var activity = Activity.idle
    public private(set) var caption: LinkStepCaption?
    public private(set) var linkedElsewhere = false

    private let linking: any AccountLinking
    private let onboarding: OnboardingAPI
    private var linkedUpstream = false
    private let storeSubmission = IdempotentSubmission()
    private let skipSubmission = IdempotentSubmission()

    public init(linking: any AccountLinking, onboarding: OnboardingAPI) {
        self.linking = linking
        self.onboarding = onboarding
    }

    public var isBusy: Bool { activity != .idle }

    public func connect() async -> LinkStepResult {
        guard !isBusy else { return .stay }
        activity = .connecting
        defer { activity = .idle }
        caption = nil
        linkedElsewhere = false
        if !linkedUpstream {
            do {
                try await linking.linkX()
            } catch {
                let linkError = error as? LinkError ?? .unknown
                linkedElsewhere = linkError == .alreadyLinkedElsewhere
                caption = LinkCopy.caption(for: linkError, linkedElsewhere: LinkCopy.xLinkedElsewhere)
                return .stay
            }
            linkedUpstream = true
        }
        return await store { [onboarding, storeSubmission] in
            try await onboarding.linkSocials(submission: storeSubmission)
        }
    }

    public func skip() async -> LinkStepResult {
        guard !isBusy else { return .stay }
        activity = .skipping
        defer { activity = .idle }
        caption = nil
        return await store { [onboarding, skipSubmission] in
            try await onboarding.skip(.socials, submission: skipSubmission)
        }
    }

    private func store(_ call: () async throws -> SessionProfile) async -> LinkStepResult {
        do {
            return .finished(try await call())
        } catch {
            let (result, caption) = LinkCopy.result(for: error)
            self.caption = caption
            return result
        }
    }
}
