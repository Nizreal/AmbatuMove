import { useMemo, useState } from "react";
import { Pressable, View } from "react-native";
import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { router } from "expo-router";
import { Button, Input, Notice, Screen, Txt, styles } from "../components/ui";
import { useStore } from "../hooks/store";
import { colors } from "../theme";

export default function Gyms() {
  const s = useStore();
  const [query, setQuery] = useState("");
  const results = useMemo(() => s.gyms.filter((gym) => `${gym.name} ${gym.address?.city ?? ""}`.toLowerCase().includes(query.trim().toLowerCase())), [s.gyms, query]);
  return (
    <Screen>
      <Txt variant="title">Direktori gym</Txt>
      <Txt color={colors.muted}>Informasi lokasi dan inventaris dari direktori AmbatuMove. Periksa detail langsung dengan gym sebelum datang.</Txt>
      <Input label="Cari nama atau kota" value={query} onChangeText={setQuery} />
      {s.gymsLoading && <Notice message="Memuat direktori gym…" />}
      {s.gymsError && <Notice error message={`Direktori gym tidak tersedia. ${s.gymsError}`} />}
      {s.gymsError && s.gyms.length > 0 && <Notice message="Lokasi yang terlihat berasal dari sinkronisasi sukses sebelumnya dan mungkin belum terbaru." />}
      {s.gymsError && <Button secondary title="Coba lagi" onPress={() => void s.refreshGyms()} />}
      {!s.gymsLoading && !s.gyms.length && !s.gymsError && <Notice message="Direktori tidak mengembalikan gym." />}
      {!!s.gyms.length && results.length === 0 && <Notice message="Tidak ada gym yang cocok dengan pencarian." />}
      {results.map((gym) => <Pressable key={gym.id} accessibilityRole="button" onPress={() => router.push({ pathname: "/gym/[id]", params: { id: gym.id } })} style={({ pressed }) => [styles.dashboardAction, pressed && { backgroundColor: colors.primarySoft }]}>
        <View style={styles.dashboardActionIcon}><MaterialCommunityIcons name="map-marker-outline" size={22} color={colors.primary} /></View>
        <View style={{ flex: 1, gap: 2 }}><Txt variant="label">{gym.name}</Txt><Txt variant="small" color={colors.muted}>{gym.address?.city || gym.address?.formatted || "Alamat belum tersedia"}</Txt><Txt variant="caption" color={colors.muted}>Status data: {gym.verificationStatus || "tidak diketahui"}</Txt></View>
        <MaterialCommunityIcons name="chevron-right" size={22} color={colors.muted} />
      </Pressable>)}
      {!s.gymsError && !s.gymsLoading && !!s.gyms.length && <Txt variant="small" color={colors.muted}>Sebagian lokasi dan inventaris dapat belum diverifikasi.</Txt>}
    </Screen>
  );
}
