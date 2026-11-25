import { useCallback, useEffect, useRef, useState } from "react";
import { View } from "react-native";
import { useLocalSearchParams } from "expo-router";
import { Button, Notice, Screen, Txt, styles } from "../../components/ui";
import { describeApiError, EquipmentOption, getGymDetails, getGymEquipment } from "../../services/ambatumove";
import { colors } from "../../theme";

export default function GymDetail() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const [gym, setGym] = useState<Record<string, unknown> | null>(null);
  const [equipment, setEquipment] = useState<EquipmentOption[]>([]);
  const [loadedForId, setLoadedForId] = useState<string | null>(null);
  const loading = loadedForId !== (id ?? "");
  const [error, setError] = useState("");
  const requestVersion = useRef(0);
  const load = useCallback(async () => {
    const version = ++requestVersion.current;
    try {
      const [detail, inventory] = await Promise.all([getGymDetails(id ?? ""), getGymEquipment(id ?? "")]);
      if (version !== requestVersion.current) return;
      setGym(detail); setEquipment(inventory);
    } catch (e) {
      if (version !== requestVersion.current) return;
      setGym(null); setEquipment([]); setError(describeApiError(e));
    } finally {
      if (version === requestVersion.current) setLoadedForId(id ?? "");
    }
  }, [id]);
  useEffect(() => {
    let alive = true;
    void Promise.resolve().then(() => { if (alive) void load(); });
    return () => { alive = false; requestVersion.current += 1; };
  }, [load]);
  if (loading) return <Screen><Notice message="Memuat detail gym dari AmbatuMove…" /></Screen>;
  if (error || !gym) return <Screen><Notice error message={`Detail gym belum tersedia. ${error}`} /><Button secondary title="Coba lagi" onPress={() => { setError(""); setLoadedForId(null); void load(); }} /></Screen>;
  const address = gym.address as Record<string, unknown> | undefined;
  const facilities = Array.isArray(gym.facilities) ? gym.facilities.map(String) : [];
  return (
    <Screen>
      <Txt variant="title">{String(gym.name ?? "Gym")}</Txt>
      <Txt color={colors.muted}>{[address?.street, address?.city, address?.region, address?.country].filter((value) => typeof value === "string" && value.length > 0).join(", ") || "Alamat belum tersedia di direktori."}</Txt>
      <View style={styles.planCard}><Txt variant="label">Status verifikasi</Txt><Txt>{String(gym.verification_status ?? "Tidak diketahui")}</Txt>{typeof gym.verified_at === "string" ? <Txt variant="small" color={colors.muted}>Diverifikasi: {new Date(gym.verified_at).toLocaleDateString()}</Txt> : <Notice message="Direktori belum mencatat waktu verifikasi." />}</View>
      <View style={styles.formSection}><Txt variant="heading">Inventaris alat</Txt>{equipment.length ? equipment.map((item) => <View key={item.id} style={styles.row}><Txt variant="label">{item.label}</Txt>{item.category ? <Txt variant="small" color={colors.muted}>{item.category}</Txt> : null}</View>) : <Notice message="Inventaris alat belum tercatat untuk lokasi ini." />}</View>
      <View style={styles.formSection}><Txt variant="heading">Fasilitas</Txt>{facilities.length ? facilities.map((facility) => <Txt key={facility}>{facility}</Txt>) : <Notice message="Informasi fasilitas belum tersedia." />}</View>
      <Notice message="Data direktori dapat belum lengkap atau belum diverifikasi. Konfirmasikan alat dan jam operasional langsung dengan gym." />
    </Screen>
  );
}
