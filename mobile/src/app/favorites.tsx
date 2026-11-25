import { useEffect, useMemo, useState } from "react";
import { router } from "expo-router";
import { View } from "react-native";
import { Button, Notice, Screen, Txt, styles } from "../components/ui";
import { ExerciseInfo, ExercisePhoto, FavoriteToggle } from "../components/exercise";
import { useStore } from "../hooks/store";
import { featureStorage } from "../services/user-features";
import { colors } from "../theme";

export default function Favorites() {
  const s = useStore();
  const [ids, setIds] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);
  useEffect(() => {
    let alive = true;
    if (!s.user) return;
    void featureStorage.favorites(s.user).then((value) => { if (alive) setIds(value); }).catch(() => { if (alive) setError("Favorit belum dapat dimuat dari penyimpanan perangkat."); }).finally(() => { if (alive) setReady(true); });
    return () => { alive = false; };
  }, [s.user]);
  const items = useMemo(() => s.catalog.filter((exercise) => ids.includes(exercise.id)), [s.catalog, ids]);
  const missing = ids.filter((id) => !s.catalog.some((exercise) => exercise.id === id));
  async function remove(id: string) {
    if (!s.user) return;
    const next = ids.filter((value) => value !== id);
    try { await featureStorage.setFavorites(s.user, next); setIds(next); setError(""); }
    catch { setError("Perubahan favorit belum tersimpan. Coba lagi."); }
  }
  return (
    <Screen>
      <Txt variant="title">Gerakan favorit</Txt>
      <Txt color={colors.muted}>Disimpan di perangkat untuk akun ini. Informasi gerakan tetap berasal dari katalog AmbatuMove.</Txt>
      {s.catalogError && <Notice error message={s.catalogError} />}
      {error && <Notice error message={error} />}
      {s.catalogLoading && <Notice message="Memuat katalog AmbatuMove…" />}
      {!ready && <Notice message="Memuat pilihan favorit…" />}
      {ready && !items.length && !missing.length && <View style={styles.emptyState}><Txt variant="heading">Belum ada favorit</Txt><Txt color={colors.muted}>Simpan gerakan dari katalog agar mudah ditemukan lagi.</Txt><Button title="Buka katalog" onPress={() => router.push("/catalog")} /></View>}
      {items.map((exercise) => <View key={exercise.id} style={styles.card}><View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between" }}><Txt variant="label">Disimpan</Txt><FavoriteToggle exercise={exercise} onChange={(favorite) => { if (!favorite) void remove(exercise.id); }} /></View><ExercisePhoto exercise={exercise} /><ExerciseInfo exercise={exercise} /><Button secondary title="Lihat panduan" onPress={() => router.push({ pathname: "/exercise/[id]", params: { id: exercise.id } })} /></View>)}
      {missing.length > 0 && <Notice message={`${missing.length} favorit tidak ada di katalog aktif saat ini dan tidak ditampilkan sebagai gerakan yang masih tersedia.`} />}
    </Screen>
  );
}
