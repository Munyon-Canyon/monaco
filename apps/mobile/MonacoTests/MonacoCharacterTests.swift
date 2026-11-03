import Testing
import UIKit

@testable import Monaco

struct MonacoCharacterTests {
    @Test func theSameSeedIsAlwaysTheSameCharacter() {
        #expect(MonacoCharacter.forSeed("user-42") == MonacoCharacter.forSeed("user-42"))
        #expect(MonacoCharacter.forSeed("Logan Norman") == MonacoCharacter.forSeed("Logan Norman"))
    }

    @Test func theSeedKeepsItsPinnedCharacter() {
        #expect(MonacoStableHash.fnv1a("") == 2_166_136_261)
        #expect(MonacoStableHash.fnv1a("a") == 3_826_002_220)
    }

    @Test func thereAreTwentyTwoCharacters() {
        #expect(MonacoCharacter.allCases.count == 22)
    }

    @Test func aCrowdWearsEveryCharacter() {
        let seen = Set((0..<2000).map { MonacoCharacter.forSeed("member-\($0)") })
        #expect(seen.count == MonacoCharacter.allCases.count)
    }

    @Test func everyCharacterIsInTheCatalog() {
        for character in MonacoCharacter.allCases {
            #expect(UIImage(named: character.imageName) != nil, "\(character.imageName) is missing")
        }
    }

    @Test func spokenNamesAreTheCharactersNames() {
        #expect(MonacoCharacter.donZorg.spokenName == "Don Zorg")
        #expect(MonacoCharacter.theBull.spokenName == "The Bull")
        #expect(MonacoCharacter.bev.spokenName == "Bev")
    }

    @Test func everyCharacterUploadsAsASmallPNG() {
        for character in MonacoCharacter.allCases {
            let data = UIImage(named: character.imageName)?.pngData()
            #expect((data?.count ?? 0) > 0, "\(character.imageName) has no PNG")
            #expect((data?.count ?? .max) < 1024 * 1024, "\(character.imageName) is too big to upload")
        }
    }
}

struct MonacoWorldSceneTests {
    @Test func theSameSeedIsAlwaysTheSameScene() {
        #expect(MonacoWorldScene.forSeed("cabal-7") == MonacoWorldScene.forSeed("cabal-7"))
    }

    @Test func aCrowdOfCabalsUsesEveryScene() {
        let seen = Set((0..<2000).map { MonacoWorldScene.forSeed("cabal-\($0)").index })
        #expect(seen.count == MonacoWorldScene.count)
    }

    @Test func everySceneIsInTheCatalog() {
        #expect(MonacoWorldScene.all.count == MonacoWorldScene.count)
        for scene in MonacoWorldScene.all {
            #expect(UIImage(named: scene.imageName) != nil, "\(scene.imageName) is missing")
        }
    }
}
