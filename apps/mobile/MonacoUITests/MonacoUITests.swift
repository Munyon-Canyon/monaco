//
//  MonacoUITests.swift
//  MonacoUITests
//
//  Created by Logan Norman on 9/15/26.
//

import XCTest

nonisolated final class MonacoUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testLaunchPerformance() throws {
        // This measures how long it takes to launch your application.
        measure(metrics: [XCTApplicationLaunchMetric()]) {
            XCUIApplication().launch()
        }
    }
}
