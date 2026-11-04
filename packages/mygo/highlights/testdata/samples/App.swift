import Foundation

/// A user
struct User: Codable, Equatable {
    let name: String
    var age: Int? = nil
}

@MainActor
func greet(_ user: User, times: Int = 1) async throws -> String {
    guard times > 0 else { throw NSError(domain: "x", code: 1) }
    return String(repeating: "hi \(user.name) ", count: times)
}
