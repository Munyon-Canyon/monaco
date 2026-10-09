import Foundation

public struct AppFeatures: Equatable, Sendable {
    public static let connectXInfoKey = "MonacoFeatureConnectX"
    public static let connectXArgument = "-MonacoFeatureConnectX"

    public var connectX: Bool

    public init(connectX: Bool = false) {
        self.connectX = connectX
    }

    public init(infoDictionary: [String: Any], arguments: [String]) {
        if let index = arguments.firstIndex(of: Self.connectXArgument), arguments.indices.contains(index + 1),
            let value = Self.flag(arguments[index + 1])
        {
            connectX = value
        } else {
            connectX = Self.flag(infoDictionary[Self.connectXInfoKey]) ?? false
        }
    }

    public static let current = AppFeatures(
        infoDictionary: Bundle.main.infoDictionary ?? [:], arguments: ProcessInfo.processInfo.arguments)

    private static func flag(_ value: Any?) -> Bool? {
        if let bool = value as? Bool { return bool }
        switch (value as? String)?.uppercased() {
        case "YES", "TRUE", "1": return true
        case "NO", "FALSE", "0": return false
        default: return nil
        }
    }
}
