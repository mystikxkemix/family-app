import Foundation

// The JSON shapes of the API (see api/internal/store/models.go).

struct User: Codable, Identifiable, Hashable, Sendable {
    let id: String
    var username: String
    var displayName: String
    /// True for the first (admin) account until it picks its own password.
    var mustChangePassword: Bool
    /// Admins manage the server from the website. Optional: older servers
    /// do not send it.
    var isAdmin: Bool?
    let createdAt: Date
}

struct Household: Codable, Hashable, Sendable {
    let id: String
    var name: String
    let createdAt: Date
}

struct MeResponse: Codable, Sendable {
    var user: User
    var household: Household
    var members: [User]
    /// False when the server has no APNs key at all.
    var pushEnabled: Bool
    /// The media tools the server can use.
    var integrations: Integrations
}

/// Which optional media tools the server has (Radarr, Sonarr, Prowlarr, TMDB).
struct Integrations: Codable, Hashable, Sendable {
    var radarr: Bool
    var sonarr: Bool
    var prowlarr: Bool
    /// TMDB: title search and the details page.
    var tmdb: Bool

    init(radarr: Bool, sonarr: Bool, prowlarr: Bool, tmdb: Bool) {
        self.radarr = radarr
        self.sonarr = sonarr
        self.prowlarr = prowlarr
        self.tmdb = tmdb
    }

    /// Every flag is optional in the JSON, so an older server (without
    /// "tmdb") still decodes.
    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        radarr = try container.decodeIfPresent(Bool.self, forKey: .radarr) ?? false
        sonarr = try container.decodeIfPresent(Bool.self, forKey: .sonarr) ?? false
        prowlarr = try container.decodeIfPresent(Bool.self, forKey: .prowlarr) ?? false
        tmdb = try container.decodeIfPresent(Bool.self, forKey: .tmdb) ?? false
    }

    /// Radarr for movies, Sonarr for shows.
    func library(for kind: MediaKind) -> Bool {
        kind == .movie ? radarr : sonarr
    }

    /// Whether the app can search titles for this kind (TMDB, else Radarr/Sonarr).
    func canSearch(_ kind: MediaKind) -> Bool {
        tmdb || library(for: kind)
    }
}

struct AuthResponse: Codable, Sendable {
    let token: String
    let user: User
}

struct InviteResponse: Codable, Sendable {
    let code: String
    let expiresAt: Date
}

struct DeviceRegistration: Codable, Sendable {
    /// False when the server has no APNs key for this app's bundle identifier.
    let supported: Bool
}

// MARK: - Lists

enum ListKind: String, Codable, CaseIterable, Identifiable, Sendable {
    case grocery, todo, other

    var id: String { rawValue }

    var title: LocalizedStringResource {
        switch self {
        case .grocery: "Groceries"
        case .todo: "To-do"
        case .other: "Other"
        }
    }

    var defaultEmoji: String {
        switch self {
        case .grocery: "🛒"
        case .todo: "✅"
        case .other: "📝"
        }
    }
}

struct SharedList: Codable, Identifiable, Hashable, Sendable {
    let id: String
    var name: String
    var kind: ListKind
    var emoji: String
    let createdBy: String
    let createdAt: Date
    var updatedAt: Date
    var openCount: Int
    var totalCount: Int

    var displayEmoji: String { emoji.isEmpty ? kind.defaultEmoji : emoji }
}

struct ListItem: Codable, Identifiable, Hashable, Sendable {
    let id: String
    let listId: String
    var text: String
    var note: String
    var checked: Bool
    var checkedBy: String?
    var checkedAt: Date?
    let createdBy: String
    let createdAt: Date
    var updatedAt: Date
}

// MARK: - Watchlist

enum MediaKind: String, Codable, CaseIterable, Identifiable, Sendable {
    case movie, show

    var id: String { rawValue }

    var title: LocalizedStringResource {
        switch self {
        case .movie: "Movie"
        case .show: "TV show"
        }
    }

    var symbol: String {
        switch self {
        case .movie: "film"
        case .show: "tv"
        }
    }
}

enum MediaStatus: String, Codable, CaseIterable, Identifiable, Sendable {
    case toWatch, watching, watched

    var id: String { rawValue }

    var title: LocalizedStringResource {
        switch self {
        case .toWatch: "To watch"
        case .watching: "Watching"
        case .watched: "Watched"
        }
    }
}

struct ExternalIDs: Codable, Hashable, Sendable {
    var tmdb: Int?
    var tvdb: Int?
    var imdb: String?
}

struct Media: Codable, Identifiable, Hashable, Sendable {
    let id: String
    var kind: MediaKind
    var title: String
    var year: Int?
    var posterUrl: String?
    var overview: String?
    var note: String
    var status: MediaStatus
    var interestedUserIds: [String]
    var externalIds: ExternalIDs
    let addedBy: String
    var watchedAt: Date?
    /// When someone sent it to Radarr or Sonarr.
    var downloadRequestedAt: Date?
    let createdAt: Date
    var updatedAt: Date
}

/// A movie or a show found by Radarr or Sonarr.
struct MediaSearchResult: Codable, Hashable, Identifiable, Sendable {
    var kind: MediaKind
    var title: String
    var year: Int?
    var overview: String?
    var posterUrl: String?
    var externalIds: ExternalIDs
    var inLibrary: Bool

    var id: String { "\(kind.rawValue)-\(externalIds.tmdb ?? 0)-\(externalIds.tvdb ?? 0)-\(title)" }
}

/// TMDB details of a watchlist title (GET watchlist/{id}/details).
struct MediaDetails: Codable, Hashable, Sendable {
    let tmdbId: Int
    var tvdbId: Int?
    var imdbId: String?
    let kind: MediaKind
    let title: String
    let originalTitle: String
    var year: Int?
    var runtimeMinutes: Int?
    var seasons: Int?
    var episodes: Int?
    let genres: [String]
    let overview: String
    /// TMDB user rating, 0 to 10.
    let tmdbRating: Double
    let tmdbVotes: Int
    /// Directors of a movie, or creators of a show.
    let directors: [String]
    let cast: [CastMember]
    var trailerUrl: String?
    var posterUrl: String?
    var backdropUrl: String?
    let tmdbUrl: String
}

struct CastMember: Codable, Hashable, Sendable {
    let name: String
    let character: String
    var photoUrl: String?
}

/// Answer of POST watchlist/{id}/download.
struct DownloadResponse: Codable, Sendable {
    let media: Media
    let alreadyInLibrary: Bool
}

/// One result of a Prowlarr interactive search.
struct Release: Codable, Hashable, Identifiable, Sendable {
    let guid: String
    let indexerId: Int
    let indexer: String?
    let title: String
    let size: Int64
    let seeders: Int?
    let leechers: Int?
    let `protocol`: String
    let publishDate: Date

    var id: String { "\(indexerId)-\(guid)" }
}

// MARK: - Places

enum PlaceCategory: String, Codable, CaseIterable, Identifiable, Sendable {
    case restaurant, bar, cafe, activity, shop, other

    var id: String { rawValue }

    var title: LocalizedStringResource {
        switch self {
        case .restaurant: "Restaurant"
        case .bar: "Bar"
        case .cafe: "Café"
        case .activity: "Activity"
        case .shop: "Shop"
        case .other: "Other"
        }
    }

    var symbol: String {
        switch self {
        case .restaurant: "fork.knife"
        case .bar: "wineglass"
        case .cafe: "cup.and.saucer"
        case .activity: "figure.walk"
        case .shop: "bag"
        case .other: "mappin"
        }
    }
}

enum PlaceStatus: String, Codable, CaseIterable, Identifiable, Sendable {
    case toTry, tried

    var id: String { rawValue }

    var title: LocalizedStringResource {
        switch self {
        case .toTry: "To try"
        case .tried: "Tried"
        }
    }
}

struct Place: Codable, Identifiable, Hashable, Sendable {
    let id: String
    var name: String
    var category: PlaceCategory
    var address: String
    var url: String
    var note: String
    var status: PlaceStatus
    /// 1 to 5 once tried, 0 when not rated.
    var rating: Int
    var interestedUserIds: [String]
    let addedBy: String
    var triedAt: Date?
    let createdAt: Date
    var updatedAt: Date
}

// MARK: - Events

/// A shared calendar entry. Named FamilyEvent to avoid confusion with EventKit.
struct FamilyEvent: Codable, Identifiable, Hashable, Sendable {
    let id: String
    var title: String
    var startsAt: Date
    var endsAt: Date?
    var allDay: Bool
    var location: String
    var notes: String
    let createdBy: String
    let createdAt: Date
    var updatedAt: Date
}
