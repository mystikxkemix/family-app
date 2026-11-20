import Foundation

/// An error from the API, with a message the user can read.
enum APIError: LocalizedError, Equatable {
    case invalidServerURL
    case insecureServerURL
    case network(String)
    case unauthorized
    case rateLimited
    case server(status: Int, code: String, message: String)
    case invalidResponse

    var errorDescription: String? {
        switch self {
        case .invalidServerURL:
            String(localized: "The server address is not valid.")
        case .insecureServerURL:
            String(localized: "The server address must start with https:// (http:// only works on the local network).")
        case .network(let message):
            String(localized: "Cannot reach the server: \(message)")
        case .unauthorized:
            String(localized: "Your session has expired. Please log in again.")
        case .rateLimited:
            String(localized: "Too many attempts. Wait a minute and try again.")
        case .server(_, let code, let message):
            Self.message(forCode: code) ?? message
        case .invalidResponse:
            String(localized: "The server sent an unexpected answer.")
        }
    }

    /// Translated messages for the error codes the user can cause.
    private static func message(forCode code: String) -> String? {
        switch code {
        case "invalid_credentials": String(localized: "Wrong username or password.")
        case "invalid_code": String(localized: "This code is not valid or has expired.")
        case "conflict": String(localized: "This username is already taken.")
        case "not_found": String(localized: "This item no longer exists.")
        case "same_password": String(localized: "The new password must be different from the current one.")
        default: nil
        }
    }
}

/// A JSON value for request bodies, so a PATCH can send only the fields that
/// changed: `["checked": true]`.
enum JSONValue: Encodable, Sendable, ExpressibleByStringLiteral, ExpressibleByBooleanLiteral, ExpressibleByIntegerLiteral {
    case string(String)
    case int(Int)
    case bool(Bool)
    case date(Date)
    case object([String: JSONValue])
    case null

    init(stringLiteral value: String) { self = .string(value) }
    init(booleanLiteral value: Bool) { self = .bool(value) }
    init(integerLiteral value: Int) { self = .int(value) }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .string(let value): try container.encode(value)
        case .int(let value): try container.encode(value)
        case .bool(let value): try container.encode(value)
        case .date(let value): try container.encode(value)
        case .object(let value): try container.encode(value)
        case .null: try container.encodeNil()
        }
    }
}

typealias JSONBody = [String: JSONValue]

/// Answer of the endpoints that return no body (204).
struct NoContent: Decodable, Sendable {}

/// A small HTTP client for the API. It is a value type: the session builds a
/// new one when the server or the token changes.
struct APIClient: Sendable {
    let baseURL: URL
    let token: String?

    /// Parses what the user typed as server address. "https://" is added when
    /// missing. Plain http is only accepted for local network addresses.
    static func serverURL(from input: String) throws -> URL {
        var text = input.trimmingCharacters(in: .whitespacesAndNewlines)
        while text.hasSuffix("/") { text.removeLast() }
        if !text.contains("://") { text = "https://" + text }
        guard let url = URL(string: text), let host = url.host(), !host.isEmpty,
              let scheme = url.scheme?.lowercased(), scheme == "https" || scheme == "http"
        else { throw APIError.invalidServerURL }
        if scheme == "http" && !isLocal(host: host) {
            throw APIError.insecureServerURL
        }
        return url
    }

    private static func isLocal(host: String) -> Bool {
        if host == "localhost" || host.hasSuffix(".local") { return true }
        let parts = host.split(separator: ".").compactMap { Int($0) }
        guard parts.count == 4 else { return false }
        return parts[0] == 10 || parts[0] == 127
            || (parts[0] == 192 && parts[1] == 168)
            || (parts[0] == 172 && (16...31).contains(parts[1]))
    }

    // MARK: Requests

    func get<Response: Decodable & Sendable>(_ path: String, query: [URLQueryItem] = []) async throws -> Response {
        try await send("GET", path, query: query, body: nil)
    }

    func post<Response: Decodable & Sendable>(_ path: String, _ body: JSONBody? = nil) async throws -> Response {
        try await send("POST", path, body: body)
    }

    func patch<Response: Decodable & Sendable>(_ path: String, _ body: JSONBody) async throws -> Response {
        try await send("PATCH", path, body: body)
    }

    func put<Response: Decodable & Sendable>(_ path: String, _ body: JSONBody) async throws -> Response {
        try await send("PUT", path, body: body)
    }

    func delete(_ path: String) async throws {
        let _: NoContent = try await send("DELETE", path, body: nil)
    }

    private func send<Response: Decodable & Sendable>(
        _ method: String, _ path: String, query: [URLQueryItem] = [], body: JSONBody?
    ) async throws -> Response {
        var url = baseURL.appending(path: "api/v1/" + path)
        if !query.isEmpty { url.append(queryItems: query) }

        var request = URLRequest(url: url, timeoutInterval: 20)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        if let body {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try Self.encoder.encode(body)
        }

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await URLSession.shared.data(for: request)
        } catch let error as URLError where error.code == .cancelled {
            // A view went away (pull-to-refresh released, screen closed): not an error.
            throw CancellationError()
        } catch {
            throw APIError.network(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else { throw APIError.invalidResponse }

        switch http.statusCode {
        case 200..<300:
            if Response.self == NoContent.self, let empty = NoContent() as? Response {
                return empty
            }
            do {
                return try Self.decoder.decode(Response.self, from: data)
            } catch {
                throw APIError.invalidResponse
            }
        case 401 where token != nil:
            throw APIError.unauthorized
        case 429:
            throw APIError.rateLimited
        default:
            let failure = try? Self.decoder.decode(ServerError.self, from: data)
            throw APIError.server(
                status: http.statusCode,
                code: failure?.code ?? "http_\(http.statusCode)",
                message: failure?.error ?? HTTPURLResponse.localizedString(forStatusCode: http.statusCode)
            )
        }
    }

    private struct ServerError: Decodable {
        let error: String
        let code: String
    }

    // MARK: JSON

    static let encoder: JSONEncoder = {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return encoder
    }()

    /// Go writes dates as RFC 3339, with or without fractional seconds.
    static let decoder: JSONDecoder = {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let text = try decoder.singleValueContainer().decode(String.self)
            if let date = try? Date(text, strategy: Date.ISO8601FormatStyle(includingFractionalSeconds: true)) {
                return date
            }
            if let date = try? Date(text, strategy: .iso8601) {
                return date
            }
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "Bad date: \(text)"))
        }
        return decoder
    }()
}
