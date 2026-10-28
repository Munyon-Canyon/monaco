import SwiftUI
import Testing

@testable import Monaco

struct MonacoToastTimingTests {
    @Test func shortSuccessKeepsTheOldDwell() {
        let dwell = MonacoToastTiming.dwell(message: "Address copied", isSuccess: true, voiceOverRunning: false)
        #expect(dwell >= 2.5)
        #expect(dwell <= 3.0)
    }

    @Test func aMoneyFailureStaysUpLongEnoughToRead() {
        let message = "We couldn't confirm that went through. Check your balance before trying again."
        let dwell = MonacoToastTiming.dwell(message: message, isSuccess: false, voiceOverRunning: false)
        // 77 characters at the old fixed 2.5s was about 18 words a second.
        #expect(dwell > 2.5)
        #expect(dwell >= 6)
    }

    @Test func everyFailureClearsTheFailureFloor() {
        let dwell = MonacoToastTiming.dwell(message: "No", isSuccess: false, voiceOverRunning: false)
        #expect(dwell == MonacoToastTiming.failureFloor)
    }

    @Test func dwellIsCappedSoAToastNeverSticks() {
        let message = String(repeating: "a", count: 4000)
        #expect(
            MonacoToastTiming.dwell(message: message, isSuccess: false, voiceOverRunning: false)
                == MonacoToastTiming.ceiling)
        #expect(
            MonacoToastTiming.dwell(message: message, isSuccess: false, voiceOverRunning: true)
                == MonacoToastTiming.voiceOverCeiling)
    }

    @Test func voiceOverGetsMoreTime() {
        let message = "Couldn't add that money. Check your connection and try again."
        let spoken = MonacoToastTiming.dwell(message: message, isSuccess: false, voiceOverRunning: true)
        let silent = MonacoToastTiming.dwell(message: message, isSuccess: false, voiceOverRunning: false)
        #expect(spoken > silent)
    }

    /// An empty message is a caller bug, but it must not produce a zero-length or negative dwell —
    /// the banner would flash and go, and `dismiss` would be handed a dwell it can never satisfy.
    @Test func anEmptyMessageStillGetsItsFloor() {
        let success = MonacoToastTiming.dwell(message: "", isSuccess: true, voiceOverRunning: false)
        let failure = MonacoToastTiming.dwell(message: "", isSuccess: false, voiceOverRunning: false)
        #expect(success == MonacoToastTiming.successFloor)
        #expect(failure == MonacoToastTiming.failureFloor)
        #expect(success > 0)
    }
}

/// The dwell countdown, and in particular the hold that used to be unbounded.
struct MonacoToastCountdownTests {
    private let slice: TimeInterval = 0.1

    /// Runs the countdown the way `dismiss` does, with a cap so a hang fails the test
    /// instead of hanging the suite. Returns the wall-clock time it took.
    private func run(
        _ countdown: inout MonacoToastCountdown,
        isHeld: @escaping (Int) -> Bool,
        limitTicks: Int = 10_000
    ) -> (elapsed: TimeInterval, finished: Bool) {
        var ticks = 0
        while !countdown.isFinished, ticks < limitTicks {
            countdown.tick(slice: slice, isHeld: isHeld(ticks))
            ticks += 1
        }
        return (Double(ticks) * slice, countdown.isFinished)
    }

    @Test func anUntouchedToastGoesAfterItsDwell() {
        var countdown = MonacoToastCountdown(dwell: 3)
        let result = run(&countdown, isHeld: { _ in false })
        #expect(result.finished)
        #expect(abs(result.elapsed - 3) < 0.15)
    }

    /// The regression. A gesture that is cancelled rather than ended — the banner torn down
    /// mid-drag, the screen pushed away, a system edge gesture winning — never delivers
    /// `onEnded`, so the hold flag stays set. The old loop only ever decremented while the flag
    /// was clear, so it slept at 10 Hz forever and pinned the toast over the money screen with
    /// tap as the only way out. The wall-clock ceiling is what ends it.
    @Test func aHoldThatNeverEndsStillReleasesTheToast() {
        let dwell: TimeInterval = 4
        var countdown = MonacoToastCountdown(dwell: dwell)
        let result = run(&countdown, isHeld: { _ in true })

        #expect(result.finished, "a toast held by a cancelled gesture never came down")
        #expect(abs(result.elapsed - (dwell + MonacoToastTiming.maximumHold)) < 0.15)
        // It was the ceiling that ended it, not the countdown: the dwell never ran.
        #expect(countdown.remaining > 0)
    }

    /// A real drag still gets its pause — the toast is not pulled out from under a moving thumb.
    @Test func aBriefHoldPostponesTheDismissal() {
        let dwell: TimeInterval = 3
        var held = MonacoToastCountdown(dwell: dwell)
        // Held for the first second, then released.
        let result = run(&held, isHeld: { $0 < 10 })

        #expect(result.finished)
        #expect(result.elapsed > dwell, "the hold bought no extra time at all")
        #expect(abs(result.elapsed - (dwell + 1)) < 0.15)
    }

    /// The ceiling is wall clock from the toast appearing, so repeated holds cannot walk it out.
    @Test func theCeilingIsNotResetByLettingGoAndGrabbingAgain() {
        let dwell: TimeInterval = 3
        var countdown = MonacoToastCountdown(dwell: dwell)
        // Held on every other tick, forever.
        let result = run(&countdown, isHeld: { $0.isMultiple(of: 2) })

        #expect(result.finished)
        #expect(result.elapsed <= dwell + MonacoToastTiming.maximumHold + 0.15)
    }
}

struct MonacoToastActionTests {
    private let now = Date(timeIntervalSinceReferenceDate: 1_000)

    private func undo(expiresIn seconds: TimeInterval? = nil) -> MonacoToastAction {
        MonacoToastAction(title: "Undo", expiresAt: seconds.map { now.addingTimeInterval($0) }) {}
    }

    @Test func aToastWithAnActionStaysAtLeastFourSeconds() {
        let plain = MonacoToastTiming.dwell(message: "Yes.", isSuccess: true, voiceOverRunning: false)
        let withAction = MonacoToastTiming.dwell(
            message: "Yes.", isSuccess: true, voiceOverRunning: false, action: undo(expiresIn: 15), now: now)
        #expect(plain == MonacoToastTiming.successFloor)
        #expect(withAction == MonacoToastTiming.actionFloor)
        #expect(MonacoToastTiming.actionFloor == 4)
    }

    @Test func theActionGoesAtItsExpiry() {
        let action = undo(expiresIn: 15)
        #expect(action.isAvailable(at: now))
        #expect(action.isAvailable(at: now.addingTimeInterval(14.9)))
        #expect(!action.isAvailable(at: now.addingTimeInterval(15)))
        #expect(undo().isAvailable(at: now.addingTimeInterval(1_000_000)))
    }

    @Test func voiceOverKeepsAnActionToastUntilItsExpiry() {
        let dwell = MonacoToastTiming.dwell(
            message: "Voted Yes. 6th of 8.", isSuccess: true, voiceOverRunning: true,
            action: undo(expiresIn: 12), now: now)
        #expect(dwell == 12)
        let long = MonacoToastTiming.dwell(
            message: "Voted Yes. 6th of 8.", isSuccess: true, voiceOverRunning: true,
            action: undo(expiresIn: 60), now: now)
        #expect(long == MonacoToastTiming.voiceOverCeiling)
    }

    @MainActor @Test func anActionRunsOnceAndDismisses() {
        var gate = MonacoToastActionGate()
        var performed = 0
        var dismissed = 0
        let first = gate.run(perform: { performed += 1 }, dismiss: { dismissed += 1 })
        let second = gate.run(perform: { performed += 1 }, dismiss: { dismissed += 1 })
        #expect(first)
        #expect(!second)
        #expect(performed == 1)
        #expect(dismissed == 1)
    }

    @Test func aTapOutsideTheActionDoesNotDismiss() {
        #expect(!MonacoToastDismissal.tapOutsideTheActionDismisses)
        #expect(MonacoToastDismissal.swipeDismisses(translation: -24.1, predicted: 0))
        #expect(!MonacoToastDismissal.swipeDismisses(translation: -10, predicted: 0))
        #expect(!MonacoToastDismissal.swipeDismisses(translation: 40, predicted: 80))
    }

    @Test func theToastSitsAtTheTop() {
        #expect(MonacoToastLayout.alignment == .top)
    }
}
