import MonacoCore
import XCTest

@MainActor
final class ProfilePhotoUploadGateTests: XCTestCase {
    private var started: [String] = []
    private var release: CheckedContinuation<Void, Never>?

    func testAPickWhileAnotherIsLoadingIsDropped() async {
        let gate = ProfilePhotoUploadGate()
        let first = Task {
            await gate.run {
                self.started.append("first")
                await withCheckedContinuation { self.release = $0 }
            }
        }
        while release == nil { await Task.yield() }
        XCTAssertTrue(gate.isUploading)

        await gate.run { self.started.append("second") }
        release?.resume()
        await first.value

        XCTAssertEqual(started, ["first"])
        XCTAssertFalse(gate.isUploading)
    }

    func testAPickAfterTheLastOneFinishedUploads() async {
        let gate = ProfilePhotoUploadGate()

        await gate.run { self.started.append("first") }
        await gate.run { self.started.append("second") }

        XCTAssertEqual(started, ["first", "second"])
        XCTAssertFalse(gate.isUploading)
    }
}
