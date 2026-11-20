import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, ApiError, sessionEvents } from './api';
import type { AuthResponse, Me, User } from './types';

interface Session {
  /** undefined while loading, null when logged out. */
  me: Me | null | undefined;
  /** True while the account must choose a new password before anything else. */
  passwordChangeRequired: boolean;
  /** The password typed at login, in memory only, for the forced change. */
  loginPassword: string | undefined;
  login: (username: string, password: string) => Promise<void>;
  register: (input: { username: string; password: string; displayName: string; inviteCode: string }) => Promise<void>;
  logout: () => Promise<void>;
  refreshMe: () => Promise<void>;
  changePassword: (current: string, next: string) => Promise<void>;
  /** Display name of a household member, or "someone". */
  memberName: (id: string, fallback: string) => string;
}

const SessionContext = createContext<Session | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null | undefined>(undefined);
  const [passwordChangeRequired, setPasswordChangeRequired] = useState(false);
  const [loginPassword, setLoginPassword] = useState<string | undefined>(undefined);

  const refreshMe = useCallback(async () => {
    try {
      const next = await api.get<Me>('me');
      setMe(next);
      setPasswordChangeRequired(next.user.mustChangePassword);
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        setMe(null);
        return;
      }
      // Network error at start: show the login page rather than a blank screen.
      setMe((current) => (current === undefined ? null : current));
      throw error;
    }
  }, []);

  useEffect(() => {
    refreshMe().catch(() => undefined);
    const onUnauthorized = () => {
      setMe(null);
      setPasswordChangeRequired(false);
    };
    const onPasswordChange = () => setPasswordChangeRequired(true);
    window.addEventListener(sessionEvents.unauthorized, onUnauthorized);
    window.addEventListener(sessionEvents.passwordChangeRequired, onPasswordChange);
    return () => {
      window.removeEventListener(sessionEvents.unauthorized, onUnauthorized);
      window.removeEventListener(sessionEvents.passwordChangeRequired, onPasswordChange);
    };
  }, [refreshMe]);

  const start = useCallback(
    async (user: User, password: string) => {
      setPasswordChangeRequired(user.mustChangePassword);
      setLoginPassword(user.mustChangePassword ? password : undefined);
      await refreshMe();
    },
    [refreshMe],
  );

  const login = useCallback(
    async (username: string, password: string) => {
      const response = await api.post<AuthResponse>('auth/login', { username, password });
      await start(response.user, password);
    },
    [start],
  );

  const register = useCallback(
    async (input: { username: string; password: string; displayName: string; inviteCode: string }) => {
      const response = await api.post<AuthResponse>('auth/register', input);
      await start(response.user, input.password);
    },
    [start],
  );

  const logout = useCallback(async () => {
    try {
      await api.post<void>('auth/logout');
    } finally {
      setMe(null);
      setPasswordChangeRequired(false);
      setLoginPassword(undefined);
    }
  }, []);

  const changePassword = useCallback(
    async (current: string, next: string) => {
      await api.post<void>('me/password', { currentPassword: current, newPassword: next });
      setPasswordChangeRequired(false);
      setLoginPassword(undefined);
      await refreshMe();
    },
    [refreshMe],
  );

  const memberName = useCallback(
    (id: string, fallback: string) => me?.members.find((member) => member.id === id)?.displayName ?? fallback,
    [me],
  );

  const value = useMemo<Session>(
    () => ({ me, passwordChangeRequired, loginPassword, login, register, logout, refreshMe, changePassword, memberName }),
    [me, passwordChangeRequired, loginPassword, login, register, logout, refreshMe, changePassword, memberName],
  );
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Session {
  const context = useContext(SessionContext);
  if (!context) throw new Error('useSession must be used inside SessionProvider');
  return context;
}

/** The logged-in data, for pages that only render after login. */
export function useMe(): Me {
  const { me } = useSession();
  if (!me) throw new Error('useMe used without a logged-in user');
  return me;
}
