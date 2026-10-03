import Foundation

struct WithdrawToBalanceRequestDTO: Encodable, Equatable {
    let shareAmountMicros: Int64?
}

struct WithdrawToBalanceJobDTO: Codable, Equatable {
    let id: String
    let status: String
    let shareUnits: Int64
    let sliceUsdc: Int64
    let payoutAddress: String?
}
