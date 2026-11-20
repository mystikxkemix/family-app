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
  createdAt: string;
  updatedAt: string;
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
  apns: { credentials: ApnsCredentialInput[] };
}

export interface TestResult {
  ok: boolean;
  message: string;
}
