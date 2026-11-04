import Foundation
import SwiftUI
import Testing

@testable import Monaco

struct QASessionResetTests {
    private let key = "MONACO_QA_SIGN_OUT_ON_LAUNCH"

    @Test func releaseBuildIgnoresTheRequest() async {
        #expect(!QASessionReset.requested(in: [key: "1"], key: nil))
    }

    @Test func debugKeyWithOneIsHonoured() async {
        #expect(QASessionReset.requested(in: [key: "1"], key: key))
    }

    @Test func otherValuesAreIgnored() async {
        #expect(!QASessionReset.requested(in: [key: "0"], key: key))
        #expect(!QASessionReset.requested(in: [:], key: key))
    }
}
