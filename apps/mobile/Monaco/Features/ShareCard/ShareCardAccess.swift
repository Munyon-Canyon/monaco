enum ShareCardAccess {
    static func canShareCabal(isMember: Bool, isActive: Bool, potLoaded: Bool) -> Bool {
        isMember && isActive && potLoaded
    }

    static func canShareMe(hasProfile: Bool, portfolioLoaded: Bool) -> Bool {
        hasProfile && portfolioLoaded
    }
}
