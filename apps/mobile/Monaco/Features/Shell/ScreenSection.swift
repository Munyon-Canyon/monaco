import SwiftUI

protocol ScreenSection<Context> {
    associatedtype Context
    associatedtype Body: View
    static var isLive: Bool { get }
    @MainActor @ViewBuilder static func body(for context: Context) -> Body
}
