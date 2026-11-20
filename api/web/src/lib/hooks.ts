import { useCallback, useEffect, useRef, useState } from 'react';

const refreshEveryMs = 30_000;

/**
 * Loads shared data and keeps it fresh: on mount, when the window gets the
 * focus back, and every 30 s while the page is visible (the other members may
 * have changed it). `setData` allows optimistic updates.
 */
export function useLoader<T>(load: () => Promise<T>, deps: unknown[] = []) {
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<unknown>(undefined);
  const loadRef = useRef(load);
  loadRef.current = load;

  const reload = useCallback(async () => {
    try {
      const next = await loadRef.current();
      setData(next);
      setError(undefined);
    } catch (failure) {
      setError(failure);
    }
  }, []);

  useEffect(() => {
    setData(undefined);
    void reload();
    // Reload when the caller's inputs change (for example the list id).
  }, deps);

  useEffect(() => {
    const refreshIfVisible = () => {
      if (document.visibilityState === 'visible') void reload();
    };
    const timer = window.setInterval(refreshIfVisible, refreshEveryMs);
    window.addEventListener('focus', refreshIfVisible);
    document.addEventListener('visibilitychange', refreshIfVisible);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener('focus', refreshIfVisible);
      document.removeEventListener('visibilitychange', refreshIfVisible);
    };
  }, [reload]);

  return { data, setData, error, reload, loading: data === undefined && error === undefined };
}

// --- Router -------------------------------------------------------------------

const navigationEvent = 'family:navigate';

/** Goes to an in-app path without reloading the page. */
export function navigate(path: string) {
  if (window.location.pathname === path) return;
  window.history.pushState(null, '', path);
  window.dispatchEvent(new Event(navigationEvent));
  window.scrollTo(0, 0);
}

/** The current path, updated on navigate() and on browser back/forward. */
export function usePath(): string {
  const [path, setPath] = useState(window.location.pathname);
  useEffect(() => {
    const update = () => setPath(window.location.pathname);
    window.addEventListener('popstate', update);
    window.addEventListener(navigationEvent, update);
    return () => {
      window.removeEventListener('popstate', update);
      window.removeEventListener(navigationEvent, update);
    };
  }, []);
  return path;
}
