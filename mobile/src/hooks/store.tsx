import React, {
  createContext,
  useContext,
  useEffect,
  useState,
  PropsWithChildren,
  useRef,
  useCallback,
} from "react";
import { Plan, Session, Constraints, defaults } from "../domain/workout";
import { keys, localHistory, read, write } from "../services/mock";
import {
  EquipmentOption,
  GymOption,
  describeApiError,
  getEquipment,
  getExercises,
  getGyms,
} from "../services/ambatumove";
type Store = {
  user: string | null;
  ready: boolean;
  error: string;
  setError: (s: string) => void;
  login: (user: string) => Promise<void>;
  logout: () => Promise<void>;
  plan: Plan | null;
  setPlan: (p: Plan | null) => void;
  active: Session | null;
  setActive: (s: Session | null) => void;
  history: Session[];
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
const loadSnapshot = () =>
  Promise.all([
    read<string | null>(keys.auth, null),
    read<Plan | null>(keys.draft, null),
    read<Session | null>(keys.active, null),
    localHistory.list(),
  ]);
export function Provider({ children }: PropsWithChildren) {
  const [user, setUser] = useState<string | null>(null),
    [ready, setReady] = useState(false),
    [error, setError] = useState(""),
    [plan, setPlanState] = useState<Plan | null>(null),
    [active, setActiveState] = useState<Session | null>(null),
    [history, setHistory] = useState<Session[]>([]),
    [catalog, setCatalog] = useState<import("../domain/workout").Exercise[]>(
      [],
    ),
    [catalogLoading, setCatalogLoading] = useState(true),
    [catalogError, setCatalogError] = useState(""),
    [equipment, setEquipment] = useState<EquipmentOption[]>([]),
    [gyms, setGyms] = useState<GymOption[]>([]),
    [gymsLoading, setGymsLoading] = useState(false),
    [gymsError, setGymsError] = useState(""),
    [conditions, setConditions] = useState(defaults);
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const persist = (key: string, value: unknown) => {
    queue.current = queue.current
      .catch(() => {})
      .then(() => write(key, value))
      .catch(() =>
        setError(
          "Perubahan belum tersimpan di perangkat. Coba lagi sebelum menutup aplikasi.",
        ),
      );
  };
  const applySnapshot = useCallback(
    ([u, p, a, h]: Awaited<ReturnType<typeof loadSnapshot>>) => {
      setUser(u);
      setPlanState(p);
      setActiveState(a && h.some((s) => s.id === a.id) ? null : a);
      setHistory(h);
      setError("");
      setReady(true);
    },
    [],
  );
  const loadError = useCallback(() => {
    setError(
      "Data lokal tidak dapat dibaca. Coba muat ulang; data tidak dihapus.",
    );
  }, []);
  const reload = async () => {
    await loadSnapshot().then(applySnapshot).catch(loadError);
  };
  useEffect(() => {
    void loadSnapshot().then(applySnapshot).catch(loadError);
  }, [applySnapshot, loadError]);
  const refreshCatalog = useCallback(async () => {
    setCatalogLoading(true);
    setCatalogError("");
    try {
      const [remoteCatalog, remoteEquipment] = await Promise.all([
        getExercises(),
        getEquipment(),
      ]);
      setCatalog(remoteCatalog);
      setEquipment(remoteEquipment);
      await Promise.all([
        write(keys.catalog, remoteCatalog),
        write(keys.equipment, remoteEquipment),
      ]);
    } catch (e) {
      setCatalogError(
        `Katalog belum dapat dimuat dari AmbatuMove. ${describeApiError(e)}`,
      );
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
      read<import("../domain/workout").Exercise[]>(keys.catalog, []).then(
        setCatalog,
      ),
      read<EquipmentOption[]>(keys.equipment, []).then(setEquipment),
      read<GymOption[]>(keys.gyms, []).then(setGyms),
    ])
      .catch(() => {})
      .finally(() => {
        void refreshCatalog();
        void refreshGyms();
      });
  }, [refreshCatalog, refreshGyms]);
  const setPlan = (p: Plan | null) => {
    setPlanState(p);
    persist(keys.draft, p);
  };
  const setActive = (s: Session | null) => {
    setActiveState(s);
    persist(keys.active, s);
  };
  return (
    <Context.Provider
      value={{
        user,
        ready,
        error,
        setError,
        plan,
        setPlan,
        active,
        setActive,
        history,
        removeHistory: async (id) => {
          await queue.current;
          setHistory(await localHistory.remove(id));
        },
        conditions,
        setConditions,
        reload,
        retrySave: async () => {
          try {
            await queue.current;
            await write(keys.draft, plan);
            await write(keys.active, active);
            setError("");
          } catch {
            setError(
              "Penyimpanan masih gagal. Pastikan perangkat memiliki ruang kosong, lalu coba lagi.",
            );
          }
        },
        catalog,
        catalogLoading,
        catalogError,
        refreshCatalog,
        equipment,
        gyms,
        gymsLoading,
        gymsError,
        refreshGyms,
        login: async (u) => {
          await write(keys.auth, u);
          setUser(u);
        },
        logout: async () => {
          await write(keys.auth, null);
          setUser(null);
        },
        save: async (s) => {
          await queue.current;
          const h = await localHistory.save(s);
          setHistory(h);
          await write(keys.active, null);
          setActiveState(null);
          setError("");
        },
      }}
    >
      {children}
    </Context.Provider>
  );
}
export const useStore = () => {
  const c = useContext(Context);
  if (!c) throw new Error("Provider missing");
  return c;
};
