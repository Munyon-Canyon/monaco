import Foundation

/// A one-time-code form, split in two: `step` is where the member is (which field is on
/// screen), `phase` is what just happened to their last request.
///
/// They used to be one enum, so any failure dropped the form back to the address field. A member
/// who tapped "Send a new code" a moment too early, and was throttled, lost the code box while
/// the first text was still arriving. Here a failure only changes the phase: the step moves back
/// to the address field when the member asks for it, and at no other time.
public struct OTPFlow: Equatable, Sendable {
    public enum Step: Equatable, Sendable {
        case enterAddress
        /// A code has been sent to `destination` and the member is typing it in.
        case enterCode(destination: String)
    }

    public enum Phase: Equatable, Sendable {
        case idle
        case sendingCode
        case awaitingCode
        case verifyingCode
        /// The last request failed. The member stays on whatever step they were on.
        case failed(message: String)
        /// The code was accepted. The form stays on the code step until the screen goes away.
        case verified
    }

    public private(set) var step: Step = .enterAddress
    public private(set) var phase: Phase = .idle

    public init() {}

    /// True while a request the member started is still in flight, or once the code went through.
    public var isBusy: Bool {
        phase == .sendingCode || phase == .verifyingCode || phase == .verified
    }

    public var isCodeEntry: Bool {
        if case .enterCode = step { return true }
        return false
    }

    public var destination: String? {
        if case .enterCode(let destination) = step { return destination }
        return nil
    }

    /// Returns false when a request is already in flight, so a second tap can't send a second code.
    public mutating func beginSend() -> Bool {
        guard !isBusy else { return false }
        phase = .sendingCode
        return true
    }

    public mutating func sendSucceeded(destination: String) {
        step = .enterCode(destination: destination)
        phase = .awaitingCode
    }

    public mutating func sendFailed(message: String) {
        phase = .failed(message: message)
    }

    public mutating func beginVerify() -> Bool {
        guard !isBusy else { return false }
        phase = .verifyingCode
        return true
    }

    public mutating func verifyFailed(message: String) {
        phase = .failed(message: message)
    }

    public mutating func verified() {
        phase = .verified
    }

    /// "Change number": the only way back to the address field. A no-op once the code went through.
    public mutating func returnToAddressEntry() {
        guard phase != .verified else { return }
        step = .enterAddress
        phase = .idle
    }
}
