@testable import MonacoAPI

extension Components.Schemas.ErrorCode {
    var isListed: Bool {
        switch self {
        case .accessRequestNotPending, .accountBanned, .accountDeleted, .accountHasBalance, .accountHasPositions,
            .accountStatusTransition, .accountSuspended, .alreadyMember, .analyticsPii, .apnsAuthFailed,
            .apnsUnavailable, .assetNotFound, .assetUntradable, .authStateTransition, .cabalBanned, .cabalNotFound,
            .cabalPaused, .calendarExpired, .cannotFollowSelf, .cannotRevokeAccess, .clientClosed, .conservationBroken,
            .dbSchemaBehind, .dbUnavailable, .decodeFailed, .displayNameInvalid, .feedItemNotFound, .forbidden,
            .handleInvalid, .handleRequired, .handleReserved, .handleTaken, .handleTooSoon, .idempotencyInFlight,
            .idempotencyMismatch, .insufficientFunds, ._internal, .invalidAddress, .invalidConfig, .invalidInput,
            .inviteExpired, .joinNeedsRequest, .jupiterRejected, .jupiterUnavailable, .leaveCreatorWithMembers,
            .leaveHoldsShares, .leaveLastMemberPotNotEmpty, .ledgerUnbalanced, .liveSwapExists, .loginMethodNotAllowed,
            .noRoute, .notAVoter, .notCabalCreator, .notCabalMember, .notFound, .notProposer, .panic, .phoneNotLinked,
            .photoInvalid, .postHogRejected, .postHogUnavailable, .potExceeded, .potValueZero, .priceUnavailable,
            .privyUnavailable, .proposalClosed, .proposalNotFound, .proposalStillOpen, .rateLimited,
            .referralCodePending, .referralCodeUnknown, .relayerUnderfunded, .requestNotNeeded, .requestPending,
            .rpcUnavailable, .sessionRequired, .slippageExceeded, .storageUnavailable, .swapFailed, .swapNotFound,
            .swapNotRetryable, .unauthorized, .upstreamTimeout, .upstreamUnavailable, .userBanned, .userNotFound,
            .versionConflict, .walletMismatch, .withdrawNotAllowed, .xNotLinked:
            true
        }
    }
}
