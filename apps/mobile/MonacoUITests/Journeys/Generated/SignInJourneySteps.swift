enum SignInJourneySteps {
    static let id = "auth/sign-in"
    static let version = 10

    static let s1p1 = JourneyStep(
        id: "S1.1",
        name: "tap Text message in the Sign-in method control",
        actor: "A",
        seconds: 5,
        copy: []
    )
    static let s1p2 = JourneyStep(
        id: "S1.2",
        name: "type smsPhoneField, then smsSendCodeButton",
        actor: "A",
        seconds: 15,
        copy: ["Send code"]
    )
    static let s1p3 = JourneyStep(id: "S1.3", name: "type smsCodeField", actor: "A", seconds: 20, copy: [])
    static let s1p4 = JourneyStep(
        id: "S1.4",
        name: "wait the session-opening screen",
        actor: "A",
        seconds: 30,
        copy: []
    )
    static let s1p5 = JourneyStep(
        id: "S1.5",
        name: "tap friends-not-now (Skip, in the toolbar)",
        actor: "A",
        seconds: 30,
        copy: []
    )
    static let s1p6 = JourneyStep(id: "S1.6", name: "wait the tab bar", actor: "A", seconds: 15, copy: [])
    static let s2p1 = JourneyStep(id: "S2.1", name: "relaunch the app", actor: "A", seconds: 15, copy: [])
    static let s2p2 = JourneyStep(id: "S2.2", name: "wait the tab bar", actor: "A", seconds: 30, copy: [])
    static let s3p1 = JourneyStep(
        id: "S3.1",
        name: "tap the Profile tab, then profile-settings-row",
        actor: "A",
        seconds: 10,
        copy: ["Settings", "Not now"]
    )
    static let s3p2 = JourneyStep(
        id: "S3.2",
        name: "tap profileSignOutButton",
        actor: "A",
        seconds: 5,
        copy: ["Sign out of Monaco?"]
    )
    static let s3p3 = JourneyStep(id: "S3.3", name: "tap profile-sign-out-confirm", actor: "A", seconds: 30, copy: [])
    static let s3p4 = JourneyStep(id: "S3.4", name: "relaunch the app", actor: "A", seconds: 30, copy: [])
    static let s3p5 = JourneyStep(
        id: "S3.5",
        name: "sign in as in S1 the Profile tab, profile-settings-row, then",
        actor: "A",
        seconds: 30,
        copy: ["Settings", "Sign out of all devices?"]
    )
    static let s3p6 = JourneyStep(
        id: "S3.6",
        name: "tap profile-sign-out-everywhere-confirm",
        actor: "A",
        seconds: 30,
        copy: []
    )
}
