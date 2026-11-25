import { useEffect, useState } from "react";
import { View } from "react-native";
import { router } from "expo-router";
import { Alert } from "../components/confirm";
import { Button, Input, Notice, Screen, Txt, styles } from "../components/ui";
import { useStore } from "../hooks/store";
import { featureStorage, WorkoutPreset } from "../services/user-features";
import { colors } from "../theme";

export default function Presets() {
  const s = useStore();
  const [name, setName] = useState("");
  const [items, setItems] = useState<WorkoutPreset[]>([]);
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);
  useEffect(() => {
    let alive = true;
    if (!s.user) return;
    void featureStorage.presets(s.user).then((values) => { if (alive) setItems(values); }).catch(() => { if (alive) setError("Preferensi tersimpan belum dapat dimuat."); }).finally(() => { if (alive) setReady(true); });
    return () => { alive = false; };
  }, [s.user]);
  async function save() {
    if (!s.user) return;
    if (!name.trim()) { setError("Masukkan nama preferensi."); return; }
    const value: WorkoutPreset = { id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`, name: name.trim().slice(0, 60), constraints: s.conditions, createdAt: new Date().toISOString() };
    const next = [value, ...items];
    try { await featureStorage.setPresets(s.user, next); setItems(next); setName(""); setError(""); }
    catch { setError("Preferensi belum tersimpan. Coba lagi."); }
  }
  async function remove(item: WorkoutPreset) {
    if (!s.user) return;
    Alert.alert("Hapus preferensi tersimpan?", item.name, [
      { text: "Batal", style: "cancel" },
      { text: "Hapus", style: "destructive", onPress: () => {
        const next = items.filter((value) => value.id !== item.id);
        void featureStorage.setPresets(s.user!, next).then(() => setItems(next)).catch(() => setError("Preferensi belum terhapus. Coba lagi."));
      } },
    ]);
  }
  return (
    <Screen>
      <Txt variant="title">Preferensi tersimpan</Txt>
      <Txt color={colors.muted}>Simpan kombinasi latihan di perangkat ini. Data tidak dikirim ke AmbatuMove sampai Anda membuat rekomendasi.</Txt>
      <View style={styles.formSection}><Input label="Nama preferensi" value={name} onChangeText={setName} maxLength={60} placeholder="Contoh: latihan pagi di rumah" /><Button title="Simpan kondisi saat ini" onPress={() => void save()} disabled={!ready} /></View>
      {error && <Notice error message={error} />}
      {!ready && <Notice message="Memuat preferensi…" />}
      {ready && !items.length && <Notice message="Belum ada preferensi tersimpan. Atur kondisi pada formulir rekomendasi, lalu simpan di sini." />}
      {items.map((item) => <View key={item.id} style={styles.card}><Txt variant="heading">{item.name}</Txt><Txt variant="small" color={colors.muted}>{item.constraints.goal} · {item.constraints.durationMinutes} menit · {item.constraints.environment} · {item.constraints.experience}</Txt><Txt variant="small">Alat: {item.constraints.equipment.join(", ") || "belum dipilih"}</Txt><View style={styles.row}><Button title="Gunakan" onPress={() => { s.setConditions(item.constraints); router.push("/recommendation/form"); }} /><Button secondary title="Hapus" onPress={() => remove(item)} /></View></View>)}
    </Screen>
  );
}
