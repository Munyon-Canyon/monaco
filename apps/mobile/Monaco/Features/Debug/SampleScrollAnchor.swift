#if DEBUG
import SwiftUI

enum SampleScrollAnchor {
    static func requested(by flag: String, in arguments: [String] = ProcessInfo.processInfo.arguments) -> UnitPoint? {
        guard let index = arguments.firstIndex(of: flag), arguments.indices.contains(index + 1) else { return nil }
        let value = arguments[index + 1]
        if value == "center" { return .leading }
        if value == "bottom" { return .bottomLeading }
        guard let fraction = Float(value), (0...1).contains(fraction) else { return nil }
        return UnitPoint(x: 0, y: CGFloat(fraction))
    }
}
#endif
