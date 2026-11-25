import React, {
  createContext,
  useContext,
  useEffect,
  useState,
  PropsWithChildren,
  useRef,
  useCallback,
} from "react";
import * as SecureStore from "expo-secure-store";
import { Platform } from "react-native";
import { Plan, Session, Constraints, defaults, upsert } from "../domain/workout";
import { keys, read, write } from "../services/storage";
import {
  ApiError,
  ApiUser,
  EquipmentOption,
  GymOption,
  TokenPair,
  WorkoutLogRecord,
  describeApiError,
  getCurrentUser,
  getEquipment,
  getExercises,
  getGyms,
  getWorkoutLogs,
  loginApi,
  logoutApi,
  refreshApiTokens,
  registerApi,
  saveCompletedSession,
} from "../services/ambatumove";

const SESSION_KEY = "ambatumove.auth.tokens.v1";
let webSession: TokenPair | null = null;

async function loadTokens(): Promise<TokenPair | null> {
  const raw = Platform.OS === "web" ? null : await SecureStore.getItemAsync(SESSION_KEY);
  const value = Platform.OS === "web" ? webSession : raw ? JSON.parse(raw) : null;
  if (!value || typeof value.access_token !== "string" || typeof value.refresh_token !== "string") return null;
  return value as TokenPair;
}

async function saveTokens(pair: TokenPair | null) {
  if (Platform.OS === "web") {
    webSession = pair;
    return;
  }
  if (pair) await SecureStore.setItemAsync(SESSION_KEY, JSON.stringify(pair));
  else await SecureStore.deleteItemAsync(SESSION_KEY);
}

const accountKeys = (email: string) => {
  const suffix = encodeURIComponent(email.trim().toLowerCase());
  return {
    history: `${keys.history}/${suffix}`,
    draft: `${keys.draft}/${suffix}`,
    active: `${keys.active}/${suffix}`,
    conditions: `@ambatumove/conditions/v1/${suffix}`,
  };
};

const loadSnapshot = (email: string) => {
  const userKeys = accountKeys(email);
  return Promise.all([
    read<Plan | null>(userKeys.draft, null),
    read<Session | null>(userKeys.active, null),
    read<Session[]>(userKeys.history, []),
    read<Constraints>(userKeys.conditions, defaults),
  ]);
};

type Store = {
  user: string | null;
  userInfo: ApiUser | null;
  ready: boolean;
  error: string;
  setError: (s: string) => void;
  login: (email: string, password: string, createAccount?: boolean) => Promise<void>;
  logout: () => Promise<void>;
  withAuth: <T>(operation: (token: string) => Promise<T>) => Promise<T>;
  plan: Plan | null;
  setPlan: (p: Plan | null) => void;
  active: Session | null;
  setActive: (s: Session | null) => void;
  history: Session[];
  remoteHistory: WorkoutLogRecord[];
  historyLoading: boolean;
  historyError: string;
  refreshHistory: () => Promise<void>;
  save: (s: Session) => Promise<void>;
  removeHistory: (id: string) => Promise<void>;
  conditions: Constraints;
  setConditions: React.Dispatch<React.SetStateAction<Constraints>>;
  reload: () => Promise<void>;
  retrySave: () => Promise<void>;
  catalog: import("../domain/workout").Exercise[];
  catalogLoading: boolean;
  catalogError: string;
  refreshCatalog: () => Promise<void>;
  equipment: EquipmentOption[];
  gyms: GymOption[];
  gymsLoading: boolean;
  gymsError: string;
  refreshGyms: () => Promise<void>;
};

const Context = createContext<Store | null>(null);

export function Provider({ children }: PropsWithChildren) {
  const [user, setUser] = useState<string | null>(null);
  const [userInfo, setUserInfo] = useState<ApiUser | null>(null);
  const [tokenPair, setTokenPair] = useState<TokenPair | null>(null);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState("");
  const [plan, setPlanState] = useState<Plan | null>(null);
  const [active, setActiveState] = useState<Session | null>(null);
  const [history, setHistory] = useState<Session[]>([]);
  const [remoteHistory, setRemoteHistory] = useState<WorkoutLogRecord[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState("");
  const [catalog, setCatalog] = useState<import("../domain/workout").Exercise[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(true);
  const [catalogError, setCatalogError] = useState("");
  const [equipment, setEquipment] = useState<EquipmentOption[]>([]);
  const [gyms, setGyms] = useState<GymOption[]>([]);
  const [gymsLoading, setGymsLoading] = useState(false);
  const [gymsError, setGymsError] = useState("");
  const [conditions, setConditions] = useState<Constraints>(defaults);
  const queue = useRef<Promise<unknown>>(Promise.resolve());

  const persist = useCallback((key: string, value: unknown) => {
    queue.current = queue.current
      .catch(() => {})
      .then(() => write(key, value))
      .catch(() => setError("Perubahan belum tersimpan di perangkat. Coba lagi sebelum menutup aplikasi."));
  }, []);

  const updateConditions = useCallback<React.Dispatch<React.SetStateAction<Constraints>>>((next) => {
    setConditions((current) => {
      const value = typeof next === "function" ? next(current) : next;
      if (JSON.stringify(current) === JSON.stringify(value)) return current;
      if (user) persist(accountKeys(user).conditions, value);
      return value;
    });
  }, [persist, user]);

  const applySnapshot = useCallback((snapshot: Awaited<ReturnType<typeof loadSnapshot>>) => {
    const [draft, activeSession, sessions, savedConditions] = snapshot;
    setPlanState(draft);
    setActiveState(activeSession && sessions.some((session) => session.id === activeSession.id) ? null : activeSession);
    setHistory(sessions);
    setConditions(savedConditions);
    setError("");
  }, []);

  const refreshPair = useCallback(async (current: TokenPair, force = false) => {
    const expiry = Date.parse(current.access_token_expires_at);
    if (!force && Number.isFinite(expiry) && expiry - Date.now() > 60_000) return current;
    const rotated = await refreshApiTokens(current.refresh_token);
    await saveTokens(rotated);
    setTokenPair(rotated);
    setUser(rotated.user.email);
    setUserInfo(rotated.user);
    return rotated;
  }, []);

  const withAuth = useCallback(async <T,>(operation: (token: string) => Promise<T>) => {
    let current = tokenPair;
    if (!current) throw new ApiError("authentication", "Masuk kembali untuk melanjutkan.");
    try {
      current = await refreshPair(current);
      return await operation(current.access_token);
    } catch (initialError) {
      if (!(initialError instanceof ApiError) || initialError.kind !== "authentication") throw initialError;
      try {
        current = await refreshPair(current, true);
        return await operation(current.access_token);
      } catch (refreshError) {
        await saveTokens(null).catch(() => {});
        setTokenPair(null);
        setUser(null);
        setUserInfo(null);
        throw refreshError;
      }
    }
  }, [tokenPair, refreshPair]);

  const refreshHistory = useCallback(async () => {
    setHistoryLoading(true);
    setHistoryError("");
    try {
      const records = await withAuth(getWorkoutLogs);
      setRemoteHistory(records);
    } catch (e) {
      setHistoryError(describeApiError(e));
    } finally {
      setHistoryLoading(false);
    }
  }, [withAuth]);

  const reload = useCallback(async () => {
    if (!user) return;
    try {
      await applySnapshot(await loadSnapshot(user));
      await refreshHistory();
    } catch {
      setError("Data lokal tidak dapat dibaca. Coba muat ulang; data tidak dihapus.");
    }
  }, [applySnapshot, refreshHistory, user]);

  useEffect(() => {
    let mounted = true;
    void (async () => {
      try {
        const saved = await loadTokens();
        if (!saved) return;
        let current = await refreshPair(saved).catch(async () => {
          const rotated = await refreshPair(saved, true);
          return rotated;
        });
        let remoteUser: ApiUser;
        try {
          remoteUser = await getCurrentUser(current.access_token);
        } catch (e) {
          if (!(e instanceof ApiError) || e.kind !== "authentication") throw e;
          current = await refreshPair(current, true);
          remoteUser = await getCurrentUser(current.access_token);
        }
        if (!mounted) return;
        setTokenPair(current);
        setUser(remoteUser.email);
        setUserInfo(remoteUser);
        await applySnapshot(await loadSnapshot(remoteUser.email));
      } catch {
        if (mounted) {
          await saveTokens(null).catch(() => {});
          setTokenPair(null);
          setUser(null);
          setUserInfo(null);
        }
      } finally {
        if (mounted) setReady(true);
      }
    })();
    return () => { mounted = false; };
  }, [applySnapshot, refreshPair]);

  const refreshCatalog = useCallback(async () => {
    setCatalogLoading(true);
    setCatalogError("");
    try {
      const [remoteCatalog, remoteEquipment] = await Promise.all([getExercises(), getEquipment()]);
      setCatalog(remoteCatalog);
      setEquipment(remoteEquipment);
      await Promise.all([write(keys.catalog, remoteCatalog), write(keys.equipment, remoteEquipment)]);
    } catch (e) {
      setCatalogError(`Katalog belum dapat dimuat dari AmbatuMove. ${describeApiError(e)}`);
    } finally {
      setCatalogLoading(false);
    }
  }, []);

  const refreshGyms = useCallback(async () => {
    setGymsLoading(true);
    setGymsError("");
    try {
      const remoteGyms = await getGyms();
      setGyms(remoteGyms);
      await write(keys.gyms, remoteGyms);
    } catch (e) {
      setGymsError(describeApiError(e));
    } finally {
      setGymsLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.all([
      read<import("../domain/workout").Exercise[]>(keys.catalog, []).then(setCatalog),
      read<EquipmentOption[]>(keys.equipment, []).then(setEquipment),
      read<GymOption[]>(keys.gyms, []).then(setGyms),
    ]).catch(() => {}).finally(() => {
      void refreshCatalog();
      void refreshGyms();
    });
  }, [refreshCatalog, refreshGyms]);

  useEffect(() => {
    if (!user) return;
    const id = setTimeout(() => { void refreshHistory(); }, 0);
    return () => clearTimeout(id);
  }, [refreshHistory, user]);

  const setPlan = (value: Plan | null) => {
    setPlanState(value);
    if (user) persist(accountKeys(user).draft, value);
  };
  const setActive = (value: Session | null) => {
    setActiveState(value);
    if (user) persist(accountKeys(user).active, value);
  };

  const store: Store = {
    user, userInfo, ready, error, setError,
    login: async (email, password, createAccount = false) => {
      const pair = createAccount ? await registerApi(email, password) : await loginApi(email, password);
      const remoteUser = await getCurrentUser(pair.access_token);
      await saveTokens(pair);
      setTokenPair(pair);
      setUser(remoteUser.email);
      setUserInfo(remoteUser);
      setRemoteHistory([]);
      await applySnapshot(await loadSnapshot(remoteUser.email));
    },
    logout: async () => {
      const current = tokenPair;
      try {
        if (current) await logoutApi(current.access_token);
      } finally {
        await saveTokens(null);
        setTokenPair(null);
        setUser(null);
        setUserInfo(null);
        setRemoteHistory([]);
        setHistoryError("");
        setPlanState(null);
        setActiveState(null);
        setHistory([]);
        setConditions(defaults);
      }
    },
    withAuth,
    plan, setPlan, active, setActive, history, remoteHistory, historyLoading, historyError,
    refreshHistory,
    removeHistory: async (id) => {
      if (!user) return;
      await queue.current;
      const next = history.filter((session) => session.id !== id);
      await write(accountKeys(user).history, next);
      setHistory(next);
    },
    conditions,
    setConditions: updateConditions,
    reload,
    retrySave: async () => {
      try {
        await queue.current;
        if (user) {
          await write(accountKeys(user).draft, plan);
          await write(accountKeys(user).active, active);
        }
        setError("");
      } catch {
        setError("Penyimpanan masih gagal. Pastikan perangkat memiliki ruang kosong, lalu coba lagi.");
      }
    },
    catalog, catalogLoading, catalogError, refreshCatalog,
    equipment, gyms, gymsLoading, gymsError, refreshGyms,
    save: async (session) => {
      if (!user) throw new ApiError("authentication", "Masuk kembali untuk menyimpan hasil.");
      await withAuth((token) => saveCompletedSession(token, session));
      await queue.current;
      const key = accountKeys(user);
      const nextHistory = upsert(await read<Session[]>(key.history, []), {
        ...session,
        savedAt: session.savedAt ?? new Date().toISOString(),
      });
      await write(key.history, nextHistory);
      await write(key.active, null);
      setHistory(nextHistory);
      setActiveState(null);
      setError("");
      await refreshHistory();
    },
  };

  return <Context.Provider value={store}>{children}</Context.Provider>;
}

export const useStore = () => {
  const context = useContext(Context);
  if (!context) throw new Error("Provider missing");
  return context;
};
