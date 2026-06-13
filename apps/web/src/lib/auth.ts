// authStore holds the access/refresh tokens in localStorage and notifies React
// (via useSyncExternalStore) when they change, so the app flips between the
// auth screen and the ledger without a manual reload.

const ACCESS_KEY = "ol.accessToken";
const REFRESH_KEY = "ol.refreshToken";

const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

export const authStore = {
  getAccess: () => localStorage.getItem(ACCESS_KEY),
  getRefresh: () => localStorage.getItem(REFRESH_KEY),
  setTokens(access: string, refresh: string) {
    localStorage.setItem(ACCESS_KEY, access);
    localStorage.setItem(REFRESH_KEY, refresh);
    emit();
  },
  clear() {
    localStorage.removeItem(ACCESS_KEY);
    localStorage.removeItem(REFRESH_KEY);
    emit();
  },
  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => listeners.delete(listener);
  },
  isAuthed: () => localStorage.getItem(ACCESS_KEY) !== null,
};
