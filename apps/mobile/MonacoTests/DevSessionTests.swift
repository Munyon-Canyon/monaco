import Testing

@testable import Monaco

#if DEBUG
@MainActor
struct DevSessionTests {
    @Test func payloadSubBecomesTheUserID() {
        let session = DevSession.fromLaunchEnvironment(["MONACO_DEV_TOKEN": "e30.eyJzdWIiOiJ1LTEifQ.sig"])

        #expect(session?.userID == "u-1")
        #expect(session?.token == "e30.eyJzdWIiOiJ1LTEifQ.sig")
    }

    @Test func missingTokenIsNil() {
        #expect(DevSession.fromLaunchEnvironment([:]) == nil)
    }

    @Test func twoPartTokenIsNil() {
        #expect(DevSession.fromLaunchEnvironment(["MONACO_DEV_TOKEN": "aaa.bbb"]) == nil)
    }

    @Test func payloadWithoutSubIsNil() {
        #expect(DevSession.fromLaunchEnvironment(["MONACO_DEV_TOKEN": "e30.eyJleHAiOjF9.sig"]) == nil)
    }
}
#endif
