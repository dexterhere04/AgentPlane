// Client-side state (§2 of the spec). Credentials and UI preferences live in
// localStorage only. No backend endpoint is involved.

const K = {
  adminToken: "agentplane.adminToken",
  userKey: "agentplane.userKey",
  theme: "agentplane.theme",
  sidebar: "agentplane.sidebar",
  setupPrefix: "agentplane.setup.",
};

const read = (k) => {
  try { return localStorage.getItem(k) || ""; } catch { return ""; }
};
const write = (k, v) => {
  try { v ? localStorage.setItem(k, v) : localStorage.removeItem(k); } catch { /* ignore */ }
};

export const getAdminToken = () => read(K.adminToken);
export const setAdminToken = (v) => write(K.adminToken, v);
export const getUserKey = () => read(K.userKey);
export const setUserKey = (v) => write(K.userKey, v);

export const getTheme = () => read(K.theme) || "light";
export const setTheme = (v) => write(K.theme, v);

export const isSidebarOpen = () => read(K.sidebar) !== "closed";
export const setSidebarOpen = (open) => write(K.sidebar, open ? "open" : "closed");

// Setup guide progress (§11). Derived from local, already-observed activity.
export const setup = {
  get(flag) { return read(K.setupPrefix + flag) === "1"; },
  set(flag) { write(K.setupPrefix + flag, "1"); },
  reset() {
    try {
      Object.keys(localStorage)
        .filter((k) => k.startsWith(K.setupPrefix))
        .forEach((k) => localStorage.removeItem(k));
    } catch { /* ignore */ }
  },
};
