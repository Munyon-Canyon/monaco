import Foundation

nonisolated enum MonacoStableHash {
    static func fnv1a(_ seed: String) -> UInt32 {
        var hash: UInt32 = 2_166_136_261
        for byte in seed.utf8 {
            hash ^= UInt32(byte)
            hash = hash &* 16_777_619
        }
        return hash
    }
}

nonisolated enum MonacoCharacter: String, CaseIterable, Sendable {
    case bev, gloria, linda, reginald
    case blip
    case donZorg = "don_zorg"
    case glorb, nova
    case biggie, bruno, peso, rex, scraps
    case harper, madison, marcus, sloane
    case dev, priya
    case theBull = "the_bull"
    case tobias, wen

    var imageName: String { "character-\(rawValue)" }

    var spokenName: String {
        switch self {
        case .donZorg: return "Don Zorg"
        case .theBull: return "The Bull"
        default: return rawValue.capitalized
        }
    }

    static func forSeed(_ seed: String) -> MonacoCharacter {
        let all = allCases
        return all[Int(MonacoStableHash.fnv1a(seed) % UInt32(all.count))]
    }
}

nonisolated struct MonacoWorldScene: Equatable, Sendable {
    static let count = 26

    let index: Int

    var imageName: String { String(format: "cover-%02d", index + 1) }

    static var all: [MonacoWorldScene] { (0..<count).map(MonacoWorldScene.init(index:)) }

    static func forSeed(_ seed: String) -> MonacoWorldScene {
        MonacoWorldScene(index: Int(MonacoStableHash.fnv1a(seed) % UInt32(count)))
    }
}
