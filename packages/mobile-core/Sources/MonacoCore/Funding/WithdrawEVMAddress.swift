import Foundation

public enum EVMAddressProblem: Error, Equatable, Sendable {
    case empty
    case notAnAddress
    case badChecksum
}

public enum EVMAddress {
    public static let hexCount = 40

    public static func validate(_ raw: String) -> Result<String, EVMAddressProblem> {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return .failure(.empty) }
        guard trimmed.hasPrefix("0x") || trimmed.hasPrefix("0X") else { return .failure(.notAnAddress) }
        let hex = String(trimmed.dropFirst(2))
        guard hex.count == hexCount, hex.allSatisfy({ $0.isASCII && $0.isHexDigit }) else {
            return .failure(.notAnAddress)
        }
        let isMixedCase = hex != hex.lowercased() && hex != hex.uppercased()
        if isMixedCase, checksummed(hex) != "0x" + hex { return .failure(.badChecksum) }
        return .success("0x" + hex)
    }

    public static func checksummed(_ hex: String) -> String {
        let lower = hex.lowercased()
        let hash = Keccak256.hash(Array(lower.utf8))
        let characters = lower.enumerated().map { index, character -> Character in
            let byte = hash[index / 2]
            let nibble = index.isMultiple(of: 2) ? byte >> 4 : byte & 0x0f
            return character.isLetter && nibble >= 8 ? Character(character.uppercased()) : character
        }
        return "0x" + String(characters)
    }

    public static func message(for problem: EVMAddressProblem, network: WithdrawNetwork) -> String {
        switch problem {
        case .empty: "Paste the \(network.name) address you want the USDC sent to."
        case .notAnAddress:
            "Check the address. It doesn't look like \(network.withArticle) address."
        case .badChecksum: "That address has a typo in it. Copy it again."
        }
    }
}

enum Keccak256 {
    private static let rate = 136

    private static let roundConstants: [UInt64] = [
        0x0000_0000_0000_0001, 0x0000_0000_0000_8082, 0x8000_0000_0000_808A, 0x8000_0000_8000_8000,
        0x0000_0000_0000_808B, 0x0000_0000_8000_0001, 0x8000_0000_8000_8081, 0x8000_0000_0000_8009,
        0x0000_0000_0000_008A, 0x0000_0000_0000_0088, 0x0000_0000_8000_8009, 0x0000_0000_8000_000A,
        0x0000_0000_8000_808B, 0x8000_0000_0000_008B, 0x8000_0000_0000_8089, 0x8000_0000_0000_8003,
        0x8000_0000_0000_8002, 0x8000_0000_0000_0080, 0x0000_0000_0000_800A, 0x8000_0000_8000_000A,
        0x8000_0000_8000_8081, 0x8000_0000_0000_8080, 0x0000_0000_8000_0001, 0x8000_0000_8000_8008,
    ]

    private static let rotations: [UInt64] = [
        0, 1, 62, 28, 27, 36, 44, 6, 55, 20, 3, 10, 43, 25, 39, 41, 45, 15, 21, 8, 18, 2, 61, 56, 14,
    ]

    static func hash(_ input: [UInt8]) -> [UInt8] {
        var message = input + [0x01]
        while !message.count.isMultiple(of: rate) { message.append(0) }
        message[message.count - 1] |= 0x80
        var state = [UInt64](repeating: 0, count: 25)
        for blockStart in stride(from: 0, to: message.count, by: rate) {
            for lane in 0..<(rate / 8) {
                var value: UInt64 = 0
                for byte in 0..<8 { value |= UInt64(message[blockStart + lane * 8 + byte]) << (8 * UInt64(byte)) }
                state[lane] ^= value
            }
            permute(&state)
        }
        return (0..<32).map { UInt8(truncatingIfNeeded: state[$0 / 8] >> (8 * UInt64($0 % 8))) }
    }

    private static func permute(_ state: inout [UInt64]) {
        for roundConstant in roundConstants {
            let columns = (0..<5).map { x in (0..<5).reduce(UInt64(0)) { $0 ^ state[x + 5 * $1] } }
            for x in 0..<5 {
                let mix = columns[(x + 4) % 5] ^ rotate(columns[(x + 1) % 5], by: 1)
                for y in 0..<5 { state[x + 5 * y] ^= mix }
            }
            var moved = [UInt64](repeating: 0, count: 25)
            for x in 0..<5 {
                for y in 0..<5 {
                    moved[y + 5 * ((2 * x + 3 * y) % 5)] = rotate(state[x + 5 * y], by: rotations[x + 5 * y])
                }
            }
            for x in 0..<5 {
                for y in 0..<5 {
                    state[x + 5 * y] = moved[x + 5 * y] ^ (~moved[(x + 1) % 5 + 5 * y] & moved[(x + 2) % 5 + 5 * y])
                }
            }
            state[0] ^= roundConstant
        }
    }

    private static func rotate(_ value: UInt64, by amount: UInt64) -> UInt64 {
        amount == 0 ? value : value << amount | value >> (64 - amount)
    }
}
