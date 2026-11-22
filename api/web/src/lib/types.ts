// JSON shapes of the API (see api/internal/store/models.go and the admin handlers).

export interface User {
  id: string;
  username: string;
  displayName: string;
  mustChangePassword: boolean;
  isAdmin: boolean;
  createdAt: string;
}

export interface Household {
  id: string;
  name: string;
  createdAt: string;
}

export interface Integrations {
  radarr: boolean;
  sonarr: boolean;
  prowlarr: boolean;
  /** TMDB: title search and the details page. */
  tmdb: boolean;
  /** Jellyfin: what we have and what we watched. */
  jellyfin: boolean;
}

export interface Me {
  user: User;
  household: Household;
  members: User[];
  pushEnabled: boolean;
  integrations: Integrations;
}

export interface AuthResponse {
  token: string;
  user: User;
}

export interface Invite {
  code: string;
  expiresAt: string;
}

// --- Lists --------------------------------------------------------------------

export type ListKind = 'grocery' | 'todo' | 'other';
export const listKinds: ListKind[] = ['grocery', 'todo', 'other'];

export interface SharedList {
  id: string;
  name: string;
  kind: ListKind;
  emoji: string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  openCount: number;
  totalCount: number;
}

export interface ListItem {
  id: string;
  listId: string;
  text: string;
  note: string;
  checked: boolean;
  checkedBy?: string;
  checkedAt?: string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

// --- Watchlist ----------------------------------------------------------------

export type MediaKind = 'movie' | 'show';
export const mediaKinds: MediaKind[] = ['movie', 'show'];
export type MediaStatus = 'toWatch' | 'watching' | 'watched';
export const mediaStatuses: MediaStatus[] = ['toWatch', 'watching', 'watched'];

export interface ExternalIds {
  tmdb?: number;
  tvdb?: number;
  imdb?: string;
}

export interface Media {
  id: string;
  kind: MediaKind;
  title: string;
  year?: number;
  posterUrl?: string;
  overview?: string;
  note: string;
  status: MediaStatus;
  interestedUserIds: string[];
  externalIds: ExternalIds;
  addedBy: string;
  watchedAt?: string;
  downloadRequestedAt?: string;
  /** "jellyfin" for a title the Jellyfin sync added. */
  source?: string;
  /** Set when the title is in Jellyfin (or was). */
  jellyfin?: MediaJellyfin;
  createdAt: string;
  updatedAt: string;
}

export interface MediaJellyfin {
  /** False once the title is no longer in Jellyfin. */
  available: boolean;
  /** Last time it was played in Jellyfin. */
  playedAt?: string;
}

export interface MediaSearchResult {
  kind: MediaKind;
  title: string;
  year?: number;
  overview?: string;
  posterUrl?: string;
  externalIds: ExternalIds;
  inLibrary: boolean;
}

export interface CastMember {
  name: string;
  character: string;
  photoUrl?: string;
}

/** Details from TMDB, in the language asked for (GET watchlist/{id}/details). */
export interface MediaDetails {
  tmdbId: number;
  tvdbId?: number;
  imdbId?: string;
  kind: MediaKind;
  title: string;
  originalTitle: string;
  year?: number;
  runtimeMinutes?: number;
  seasons?: number;
  episodes?: number;
  genres: string[];
  overview: string;
  /** TMDB users' average, 0 to 10. */
  tmdbRating: number;
  tmdbVotes: number;
  /** Directors for a movie, creators for a show. */
  directors: string[];
  cast: CastMember[];
  trailerUrl?: string;
  posterUrl?: string;
  backdropUrl?: string;
  tmdbUrl: string;
}

export interface DownloadResponse {
  media: Media;
  alreadyInLibrary: boolean;
}

export interface Release {
  guid: string;
  indexerId: number;
  indexer?: string;
  title: string;
  size: number;
  seeders?: number;
  leechers?: number;
  protocol: string;
  publishDate: string;
}

// --- Places -------------------------------------------------------------------

export type PlaceCategory = 'restaurant' | 'bar' | 'cafe' | 'activity' | 'shop' | 'other';
export const placeCategories: PlaceCategory[] = ['restaurant', 'bar', 'cafe', 'activity', 'shop', 'other'];
export type PlaceStatus = 'toTry' | 'tried';
export const placeStatuses: PlaceStatus[] = ['toTry', 'tried'];

export interface Place {
  id: string;
  name: string;
  category: PlaceCategory;
  address: string;
  url: string;
  note: string;
  status: PlaceStatus;
  rating: number;
  interestedUserIds: string[];
  addedBy: string;
  triedAt?: string;
  createdAt: string;
  updatedAt: string;
}

// --- Events -------------------------------------------------------------------

export interface FamilyEvent {
  id: string;
  title: string;
  startsAt: string;
  endsAt?: string;
  allDay: boolean;
  location: string;
  notes: string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

// --- Admin --------------------------------------------------------------------

export interface AdminStatus {
  version: string;
  startedAt: string;
  users: number;
  households: number;
  pushEnabled: boolean;
  integrations: Integrations;
  settingsFile: string;
  settingsFound: boolean;
  settingsWritable: boolean;
}

export interface AdminUser {
  id: string;
  username: string;
  displayName: string;
  isAdmin: boolean;
  mustChangePassword: boolean;
  createdAt: string;
  householdName: string;
}

export type ArrTool = 'radarr' | 'sonarr' | 'prowlarr';
/** Tools with a connection test in the admin page. */
export type TestableTool = ArrTool | 'tmdb' | 'jellyfin';

/** One media tool as the API shows it: the key itself is never sent back. */
export interface ArrSettings {
  url: string;
  apiKeySet: boolean;
  qualityProfileId?: number;
  rootFolder?: string;
}

export interface ApnsCredential {
  teamId: string;
  keyId: string;
  topics: string[];
  keySet: boolean;
}

export interface AdminSettings {
  radarr: ArrSettings;
  sonarr: ArrSettings;
  prowlarr: ArrSettings;
  tmdb: { apiKeySet: boolean };
  jellyfin: { url: string; apiKeySet: boolean };
  apns: { credentials: ApnsCredential[] };
}

/** What the admin sends: "" for apiKey / key keeps the saved secret. */
export interface ArrSettingsInput {
  url: string;
  apiKey: string;
  qualityProfileId?: number;
  rootFolder?: string;
}

export interface ApnsCredentialInput {
  teamId: string;
  keyId: string;
  topics: string[];
  key: string;
}

export interface AdminSettingsInput {
  radarr: ArrSettingsInput;
  sonarr: ArrSettingsInput;
  prowlarr: ArrSettingsInput;
  /** "" keeps the saved key; remove deletes it. */
  tmdb: { apiKey: string; remove?: boolean };
  /** "" for url turns Jellyfin off; "" for apiKey keeps the saved key. */
  jellyfin: { url: string; apiKey: string };
  apns: { credentials: ApnsCredentialInput[] };
}

/** A Jellyfin account (GET admin/jellyfin/users). */
export interface JellyfinUser {
  id: string;
  name: string;
}

/** The result of one Jellyfin sync. */
export interface JellyfinSyncResult {
  at: string;
  ok: boolean;
  message?: string;
  households: number;
  /** Jellyfin movies and shows read. */
  available: number;
  /** Watchlist titles linked to Jellyfin. */
  matched: number;
  markedWatching: number;
  markedWatched: number;
  imported: number;
  gone: number;
  durationMs: number;
}

/** The Jellyfin card of the admin page (GET admin/jellyfin). */
export interface JellyfinState {
  configured: boolean;
  /** The Jellyfin account of the admin's household, "" when none. */
  jellyfinUserId: string;
  running: boolean;
  lastSync: JellyfinSyncResult | null;
}

export interface TestResult {
  ok: boolean;
  message: string;
}

/** One row of Admin > Connections (GET admin/connections). */
export interface ConnectionResult {
  service: string;
  configured: boolean;
  ok: boolean;
  /** Version on success, reason on failure. */
  message: string;
  latencyMs: number;
}
