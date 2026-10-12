#if DEBUG
import Foundation

enum ShareCardSampleSeries {
    static func permille(gain: Bool) -> [(TimeInterval, Int64)] {
        (0..<48).map { index in
            let t = Double(index) / 47
            let wobble = (sin(t * 11) * 0.07 + sin(t * 27) * 0.025) * (1 - t)
            return (t * 31_536_000, Int64(((pow(t, gain ? 2.2 : 1) + wobble) * 1000).rounded()))
        }
    }
}
#endif
