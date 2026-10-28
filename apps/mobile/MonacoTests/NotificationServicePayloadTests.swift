import Foundation
import Testing

@testable import Monaco

@MainActor
struct NotificationServicePayloadTests {
    private let hosts: Set<String> = ["project.supabase.co"]
    private let avatar = "https://project.supabase.co/storage/v1/object/public/avatars/"

    private func person() -> [AnyHashable: Any] {
        [
            "aps": ["thread-id": "cabal-c1", "mutable-content": 1],
            "sender_kind": "person",
            "sender_id": "u1",
            "sender_name": "Mia",
            "sender_image_url": avatar + "mia.png",
            "group_id": "c1",
            "group_name": "AC Capital",
            "group_image_url": avatar + "ac.png",
        ]
    }

    @Test func aPersonLedPushKeepsTheSenderTheGroupAndTheThread() throws {
        let payload = try #require(PushSenderPayload(userInfo: person(), allowedHosts: hosts))
        #expect(payload.kind == .person)
        #expect(payload.senderID == "u1")
        #expect(payload.senderName == "Mia")
        #expect(payload.senderImageURL?.absoluteString == avatar + "mia.png")
        #expect(payload.group == .init(id: "c1", name: "AC Capital", imageURL: URL(string: avatar + "ac.png")))
        #expect(payload.threadID == "cabal-c1")
    }

    @Test func aCabalLedPushIsBothTheSenderAndTheGroup() throws {
        let info: [AnyHashable: Any] = [
            "aps": ["thread-id": "cabal-c1"],
            "sender_kind": "cabal",
            "sender_id": "c1",
            "sender_name": "AC Capital",
            "sender_image_url": avatar + "ac.png",
        ]
        let payload = try #require(PushSenderPayload(userInfo: info, allowedHosts: hosts))
        #expect(payload.kind == .cabal)
        #expect(payload.group == .init(id: "c1", name: "AC Capital", imageURL: URL(string: avatar + "ac.png")))
    }

    @Test func anAppPushIsLeftUntouched() {
        var info = person()
        info["sender_kind"] = "app"
        #expect(PushSenderPayload(userInfo: info, allowedHosts: hosts) == nil)
    }

    @Test func aPushWithoutSenderKeysIsLeftUntouched() {
        #expect(PushSenderPayload(userInfo: ["aps": ["alert": "x"]], allowedHosts: hosts) == nil)
    }

    @Test func aMissingNameIsLeftUntouched() {
        var info = person()
        info["sender_name"] = nil
        #expect(PushSenderPayload(userInfo: info, allowedHosts: hosts) == nil)
        info["sender_name"] = ""
        #expect(PushSenderPayload(userInfo: info, allowedHosts: hosts) == nil)
    }

    @Test func anHttpPictureIsDroppedButThePushStillLeadsWithTheName() throws {
        var info = person()
        info["sender_image_url"] = "http://project.supabase.co/storage/v1/object/public/avatars/mia.png"
        let payload = try #require(PushSenderPayload(userInfo: info, allowedHosts: hosts))
        #expect(payload.senderImageURL == nil)
        #expect(payload.senderName == "Mia")
        #expect(payload.group?.imageURL != nil)
    }

    @Test func aForeignHostPictureIsDropped() throws {
        var info = person()
        info["sender_image_url"] = "https://evil.example.com/mia.png"
        info["group_image_url"] = "https://project.supabase.co.evil.example.com/ac.png"
        let payload = try #require(PushSenderPayload(userInfo: info, allowedHosts: hosts))
        #expect(payload.senderImageURL == nil)
        #expect(payload.group?.imageURL == nil)
    }

    @Test func noConfiguredHostsMeansNoPictures() throws {
        let payload = try #require(PushSenderPayload(userInfo: person(), allowedHosts: []))
        #expect(payload.senderImageURL == nil)
    }

    @Test func theThreadFallsBackToTheGroupThenTheSender() throws {
        var info = person()
        info["aps"] = [:]
        #expect(try #require(PushSenderPayload(userInfo: info, allowedHosts: hosts)).threadID == "cabal-c1")
        info["group_id"] = nil
        #expect(try #require(PushSenderPayload(userInfo: info, allowedHosts: hosts)).threadID == "u1")
    }

    @Test func hostsComeFromTheInfoPlistValue() {
        #expect(
            PushSenderPayload.allowedHosts(in: ["MonacoImageHosts": " A.supabase.co, b.example.com ,"])
                == ["a.supabase.co", "b.example.com"])
        #expect(PushSenderPayload.allowedHosts(in: ["MonacoImageHosts": "$(MONACO_IMAGE_HOSTS)"]).isEmpty)
        #expect(PushSenderPayload.allowedHosts(in: [:]).isEmpty)
        #expect(PushSenderPayload.allowedHosts(in: nil).isEmpty)
    }
}
