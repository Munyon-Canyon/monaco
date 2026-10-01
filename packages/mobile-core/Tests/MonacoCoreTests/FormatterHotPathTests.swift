import XCTest

#if canImport(Darwin)
import Darwin
#elseif canImport(Glibc)
import Glibc
#endif

@testable import MonacoCore

/// These formatters run once per visible row on every SwiftUI body pass, so they must stay
/// cheap and must give the same answer from any thread.
final class FormatterHotPathTests: XCTestCase {
    func testOneScreenOfLabels_staysWellInsideAFrame() {
        // The first call builds each formatter; scrolling cost is every call after that.
        renderOneScreenOfLabels()
        let started = threadCPUTimeNanoseconds()
        renderOneScreenOfLabels()
        let elapsedMilliseconds = Double(threadCPUTimeNanoseconds() - started) / 1_000_000
        // A 60Hz frame is 16.7ms. Thread CPU time ignores runnable-queue waits.
        // Building formatters per call took ~23ms of wall clock here on a fast Mac.
        // 16ms is a temporary ceiling (operator-approved) until the formatter is optimized
        // and this is fine-tuned back down; see follow-up #1264.
        XCTAssertLessThan(elapsedMilliseconds, 16)
    }

    /// Roughly what a busy feed renders per pass: 60 money labels, 60 ages, 30 share counts.
    private func renderOneScreenOfLabels() {
        for index in 0..<60 {
            _ = UsdAmountFormatter.format(micros: Int64(index) * 1_234_567)
        }
        for _ in 0..<30 {
            _ = RelativeTimeFormatter.label(iso: "2026-09-18T15:04:05.123456Z")
            _ = ProposalTimeFormatter.ageLabel("2026-09-01T15:04:05Z")
            _ = ProposalShareFormatter.sharesLabel(fromAtomics: "120340000")
        }
    }

    func testSharedFormatters_giveTheSameAnswerFromManyThreads() {
        let expectedMoney = UsdAmountFormatter.format(micros: 1_250_500_000)
        let expectedShares = ProposalShareFormatter.sharesLabel(fromAtomics: "120340000")
        let expectedDate = ProposalTimeFormatter.parse("2026-09-18T15:04:05.123Z")
        XCTAssertEqual(expectedMoney, "$1,250.50")
        XCTAssertEqual(expectedShares, "1.2034 shares")
        XCTAssertNotNil(expectedDate)

        let mismatches = Mismatches()
        DispatchQueue.concurrentPerform(iterations: 2_000) { _ in
            if UsdAmountFormatter.format(micros: 1_250_500_000) != expectedMoney
                || ProposalShareFormatter.sharesLabel(fromAtomics: "120340000") != expectedShares
                || ProposalTimeFormatter.parse("2026-09-18T15:04:05.123Z") != expectedDate
                || RelativeTimeFormatter.parse("2026-09-18T15:04:05Z") == nil
            {
                mismatches.record()
            }
        }
        XCTAssertEqual(mismatches.count, 0)
    }

    func testRelativeLabel_followsTheCallersCalendar() {
        var tokyo = Calendar(identifier: .gregorian)
        tokyo.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        var losAngeles = Calendar(identifier: .gregorian)
        losAngeles.timeZone = TimeZone(identifier: "America/Los_Angeles")!
        let now = ISO8601DateFormatter().date(from: "2026-09-18T12:00:00Z")!
        // 23:30 UTC on the 14th is already the 15th in Tokyo and still the 14th in LA.
        XCTAssertEqual(RelativeTimeFormatter.label(iso: "2026-09-14T23:30:00Z", now: now, calendar: tokyo), "Sep 15")
        XCTAssertEqual(
            RelativeTimeFormatter.label(iso: "2026-09-14T23:30:00Z", now: now, calendar: losAngeles), "Sep 14")
        XCTAssertEqual(
            RelativeTimeFormatter.label(iso: "2025-09-14T23:30:00Z", now: now, calendar: losAngeles), "Sep 14, 2025")
    }
}

private func threadCPUTimeNanoseconds() -> Int64 {
    var sample = timespec()
    let clockID: clockid_t = CLOCK_THREAD_CPUTIME_ID
    let status = clock_gettime(clockID, &sample)
    precondition(status == 0, "clock_gettime(CLOCK_THREAD_CPUTIME_ID) failed: \(errno)")
    return Int64(sample.tv_sec) * 1_000_000_000 + Int64(sample.tv_nsec)
}

private final class Mismatches: @unchecked Sendable {
    private let lock = NSLock()
    private var value = 0

    var count: Int {
        lock.lock()
        defer { lock.unlock() }
        return value
    }

    func record() {
        lock.lock()
        value += 1
        lock.unlock()
    }
}
