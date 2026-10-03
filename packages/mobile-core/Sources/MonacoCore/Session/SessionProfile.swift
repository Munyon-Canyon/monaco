import Foundation
import MonacoAPI

public struct SessionProfile: Equatable, Sendable {
    public enum AuthState: Equatable, Sendable {
        case created
        case awaitingPhone
        case awaitingSocials
        case onboardingCompleted
        case unknown(String)
    }

    public enum AccountStatus: Equatable, Sendable {
        case active
        case suspended
        case banned
        case unknown(String)
    }

    public var userID: String
    public var handle: String?
    public var displayName: String
    public var photoURL: URL?
    public var authState: AuthState
    public var accountStatus: AccountStatus
    public var memberWalletAddress: String
    public var phoneLinked: Bool
    public var xUsername: String?
    public var handleChangeableAt: Date?
    public var createdAt: Date

    public init(
        userID: String,
        handle: String?,
        displayName: String,
        photoURL: URL?,
        authState: AuthState,
        accountStatus: AccountStatus,
        memberWalletAddress: String,
        phoneLinked: Bool,
        xUsername: String?,
        handleChangeableAt: Date?,
        createdAt: Date
    ) {
        self.userID = userID
        self.handle = handle
        self.displayName = displayName
        self.photoURL = photoURL
        self.authState = authState
        self.accountStatus = accountStatus
        self.memberWalletAddress = memberWalletAddress
        self.phoneLinked = phoneLinked
        self.xUsername = xUsername
        self.handleChangeableAt = handleChangeableAt
        self.createdAt = createdAt
    }

    public func withDisplayName(_ name: String) -> SessionProfile {
        var copy = self
        copy.displayName = name
        return copy
    }

    public init(_ me: Components.Schemas.Me) {
        self.init(
            userID: me.id,
            handle: me.handle,
            displayName: me.displayName,
            photoURL: Self.photoURL(me.photoUrl),
            authState: AuthState(me.authState),
            accountStatus: AccountStatus(me.accountStatus),
            memberWalletAddress: me.memberWalletAddress,
            phoneLinked: me.phoneLinked,
            xUsername: me.xUsername,
            handleChangeableAt: me.handleChangeableAt,
            createdAt: me.createdAt
        )
    }

    public init(json: Data) throws {
        let wire: WireMe
        do {
            wire = try JSONDecoder().decode(WireMe.self, from: json)
        } catch {
            throw APIError.decoding(String(describing: error))
        }
        guard let createdAt = SharedFormatters.iso8601Date(from: wire.createdAt) else {
            throw APIError.decoding("created_at")
        }
        let handleChangeableAt: Date?
        if let raw = wire.handleChangeableAt {
            guard let parsed = SharedFormatters.iso8601Date(from: raw) else {
                throw APIError.decoding("handle_changeable_at")
            }
            handleChangeableAt = parsed
        } else {
            handleChangeableAt = nil
        }
        self.init(
            userID: wire.id,
            handle: wire.handle,
            displayName: wire.displayName,
            photoURL: Self.photoURL(wire.photoUrl),
            authState: AuthState(wire: wire.authState),
            accountStatus: AccountStatus(wire: wire.accountStatus),
            memberWalletAddress: wire.memberWalletAddress,
            phoneLinked: wire.phoneLinked,
            xUsername: wire.xUsername,
            handleChangeableAt: handleChangeableAt,
            createdAt: createdAt
        )
    }

    private static func photoURL(_ raw: String?) -> URL? {
        guard let raw, let url = URL(string: raw), url.scheme != nil else { return nil }
        return url
    }
}

extension SessionProfile.AuthState {
    init(_ value: Components.Schemas.AuthState) {
        switch value {
        case .created: self = .created
        case .awaitingPhone: self = .awaitingPhone
        case .awaitingSocials: self = .awaitingSocials
        case .onboardingCompleted: self = .onboardingCompleted
        }
    }

    init(wire: String) {
        switch wire {
        case "CREATED": self = .created
        case "AWAITING_PHONE": self = .awaitingPhone
        case "AWAITING_SOCIALS": self = .awaitingSocials
        case "ONBOARDING_COMPLETED": self = .onboardingCompleted
        default: self = .unknown(wire)
        }
    }
}

extension SessionProfile.AccountStatus {
    init(_ value: Components.Schemas.AccountStatus) {
        switch value {
        case .active: self = .active
        case .suspended: self = .suspended
        case .banned: self = .banned
        }
    }

    init(wire: String) {
        switch wire {
        case "active": self = .active
        case "suspended": self = .suspended
        case "banned": self = .banned
        default: self = .unknown(wire)
        }
    }
}

private struct WireMe: Decodable {
    var id: String
    var handle: String?
    var displayName: String
    var photoUrl: String?
    var authState: String
    var accountStatus: String
    var memberWalletAddress: String
    var phoneLinked: Bool
    var xUsername: String?
    var handleChangeableAt: String?
    var createdAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case handle
        case displayName = "display_name"
        case photoUrl = "photo_url"
        case authState = "auth_state"
        case accountStatus = "account_status"
        case memberWalletAddress = "member_wallet_address"
        case phoneLinked = "phone_linked"
        case xUsername = "x_username"
        case handleChangeableAt = "handle_changeable_at"
        case createdAt = "created_at"
    }
}
