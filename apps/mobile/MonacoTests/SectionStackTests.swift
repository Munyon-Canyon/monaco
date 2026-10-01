import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct SectionStackTests {
    @Test func liveFiltersAndKeepsArrayOrder() {
        let result = SectionStack<Void>.live([TestSectionC.self, TestSectionB.self, TestSectionA.self])

        #expect(result.map { String(describing: $0) } == ["TestSectionC", "TestSectionA"])
    }
}

private protocol CabalSection: ScreenSection where Context == Void {}

private enum TestSectionA: @MainActor CabalSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        Text("A")
    }
}

private enum TestSectionB: @MainActor CabalSection {
    static let isLive = false

    static func body(for context: Void) -> some View {
        Text("B")
    }
}

private enum TestSectionC: @MainActor CabalSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        Text("C")
    }
}
